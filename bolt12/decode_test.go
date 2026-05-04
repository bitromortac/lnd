package bolt12

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
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

// TestDecodeRejectsOversizedRecord pins the per-record cap by feeding
// the decoder a TLV declaring a length one byte over tlv.MaxRecordSize.
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
