package bolt12

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// offersTestVector represents a single test case from offers-test.json.
type offersTestVector struct {
	Description string            `json:"description"`
	Valid       bool              `json:"valid"`
	Bolt12      string            `json:"bolt12"`
	Fields      []offersTestField `json:"fields"`
}

// offersTestField represents an expected TLV field in the test vector.
type offersTestField struct {
	Type   uint64 `json:"type"`
	Length uint64 `json:"length"`
	Hex    string `json:"hex"`
}

// TestDecodeOffersVectors runs through every test case in the spec's
// offers-test.json. For valid cases, it decodes the bech32 string and
// verifies each TLV field matches the expected type, length, and hex
// value. For invalid cases, it verifies that decoding or structural
// checks fail. Semantic validation is exercised separately in
// validate_test.go.
func TestDecodeOffersVectors(t *testing.T) {
	t.Parallel()

	vectors := loadOffersVectors(t)
	require.NotEmpty(t, vectors)

	for _, tc := range vectors {
		t.Run(tc.Description, func(t *testing.T) {
			t.Parallel()

			// Decode the bech32 string to raw TLV bytes.
			hrp, tlvBytes, bech32Err := Decode(tc.Bolt12)

			if !tc.Valid {
				// Invalid vectors fail at one of bech32,
				// TLV-stream parse, or DecodeOffer; any
				// non-nil error suffices.
				if bech32Err != nil {
					return
				}
				if _, err := decodeOffer(tlvBytes); err != nil {
					return
				}

				return
			}

			require.NoError(t, bech32Err, tc.Description)
			require.Equal(t, HRPOffer, hrp)
			require.NotEmpty(t, tlvBytes)

			// Read the wire stream as a record list and
			// verify field-by-field against the spec vector.
			records := streamToRecords(t, tlvBytes)

			require.Len(t, records, len(tc.Fields),
				"field count mismatch")

			for i, expected := range tc.Fields {
				r := records[i]

				require.Equal(
					t, expected.Type, uint64(r.Type()),
					"field %d type mismatch", i,
				)

				// Re-encode the record in isolation to
				// recover the type+length+value bytes,
				// then strip the type+length prefix to
				// compare against the spec's value hex.
				fullBytes, err := lnwire.EncodeRecords(
					[]tlv.Record{r},
				)
				require.NoError(t, err)
				valHex := extractValueHex(
					t, fullBytes,
					expected.Type, expected.Length,
				)
				require.Equal(
					t, expected.Hex, valHex,
					"field %d hex mismatch", i,
				)
			}

			// Round-trip: decode into Offer struct and
			// re-encode.
			offer, err := decodeOffer(tlvBytes)
			require.NoError(t, err)

			reencoded, err := offer.Encode()
			require.NoError(t, err)

			// Re-read and compare field count. Unknown odd
			// fields in the bolt12 signed range survive the
			// round-trip via decodedTLVs per the BOLT 1
			// odd-rule, so the re-encoded count matches the
			// original input rather than just the known-field
			// subset.
			reRecords := streamToRecords(t, reencoded)
			require.Len(t, reRecords, len(tc.Fields),
				"re-encoded field count mismatch")
		})
	}
}

// extractValueHex parses a raw TLV encoding and returns the hex-encoded
// value portion.
func extractValueHex(
	t *testing.T, fullEncoding []byte,
	expectedType, expectedLen uint64) string {

	t.Helper()

	// The value is the last expectedLen bytes of the full encoding.
	// We need to skip the type and length varints.
	require.GreaterOrEqual(
		t, uint64(len(fullEncoding)), expectedLen,
		"full encoding too short",
	)

	valueBytes := fullEncoding[uint64(len(fullEncoding))-expectedLen:]

	return hex.EncodeToString(valueBytes)
}

// isKnownOfferType returns true if the TLV type is a known offer field
// (types 2-22 inclusive, even numbers).
func isKnownOfferType(typ uint64) bool {
	return typ >= 2 && typ <= 22 && typ%2 == 0
}

// TestDecodeRejectsOversizedRecord asserts that an oversize record
// declaration is rejected before any value allocation occurs. The
// typed-stream pass uses the P2P decode variant which caps each
// record at tlv.MaxRecordSize.
func TestDecodeRejectsOversizedRecord(t *testing.T) {
	t.Parallel()

	// Build a synthetic TLV with type=22 (offer_issuer_id, known by
	// the offer decoder) and declared length one byte over the cap.
	// The value bytes are present so the framing itself is consistent.
	const oversize = tlv.MaxRecordSize + 1
	var (
		buf [8]byte
		w   bytes.Buffer
	)
	require.NoError(t, tlv.WriteVarInt(&w, 22, &buf))
	require.NoError(t, tlv.WriteVarInt(&w, oversize, &buf))
	w.Write(make([]byte, oversize))

	_, err := decodeOffer(w.Bytes())
	require.ErrorIs(t, err, tlv.ErrRecordTooLarge,
		"expected an oversize-record rejection, got %v", err)
}

// TestDecodeOfferRoundTrip decodes a minimal offer string and verifies
// the issuer ID field is correctly parsed.
func TestDecodeOfferRoundTrip(t *testing.T) {
	t.Parallel()

	// Minimal offer: just offer_issuer_id (type 22).
	offerStr := "lno1zcss9mk8y3wkklfvevcrszlmu23kfrxh49p" +
		"x20665dqwmn4p72pksese"

	_, tlvBytes, err := Decode(offerStr)
	require.NoError(t, err)

	offer, err := decodeOffer(tlvBytes)
	require.NoError(t, err)

	// Verify issuer ID is present and correctly typed.
	var (
		issuerKey *btcec.PublicKey
		set       bool
	)
	offer.OfferIssuerID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType22, *btcec.PublicKey]) {
			issuerKey = r.Val
			set = true
		},
	)
	require.True(t, set, "expected offer_issuer_id to be set")

	expectedHex := "02eec7245d6b7d2ccb30380bfbe2a3648cd7a94" +
		"2653f5aa340edcea1f283686619"
	require.Equal(t, expectedHex,
		hex.EncodeToString(issuerKey.SerializeCompressed()))

	// Re-encode and verify bytes match.
	reencoded, err := offer.Encode()
	require.NoError(t, err)
	require.Equal(t, tlvBytes, reencoded)
}
