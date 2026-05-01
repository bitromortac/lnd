package bolt12

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// TestDecodeOfferString tests the top-level convenience function with a
// valid offer from the spec test vectors.
func TestDecodeOfferString(t *testing.T) {
	t.Parallel()

	// "with description (but no amount)" from offers-test.json.
	vec := findTestVector(t, "with description (but no amount)")

	offer, err := DecodeOfferString(
		vec.Bolt12, farFutureNow(), bitcoinMainnetGenesisHash,
	)
	require.NoError(t, err)

	var desc []byte
	offer.OfferDescription.WhenSome(
		func(r tlv.RecordT[tlv.TlvType10, tlv.Blob]) {
			desc = r.Val
		},
	)
	require.Equal(t, "Test vectors", string(desc))
}

// TestDecodeOfferStringRejectsInvalid asserts that DecodeOfferString
// itself rejects an offer that fails a reader MUST gate — the wrapper
// folds ValidateOfferRead in, so semantically invalid offers never reach
// the caller.
func TestDecodeOfferStringRejectsInvalid(t *testing.T) {
	t.Parallel()

	// Missing issuer ID and no paths.
	vec := findTestVector(
		t, "Missing offer_issuer_id and no offer_path",
	)
	_, err := DecodeOfferString(
		vec.Bolt12, farFutureNow(), bitcoinMainnetGenesisHash,
	)
	require.Error(t, err)
}

// TestDecodeOfferStringWrongHRP verifies that an lnr string is rejected
// by DecodeOfferString.
func TestDecodeOfferStringWrongHRP(t *testing.T) {
	t.Parallel()

	// Use the invoice_request string from signature-test.json.
	lnrStr := "lnr1qqyqqqqqqqqqqqqqqcp4256ypqqkgzshgysy6ct" +
		"5dpjk6ct5d93kzmpq23ex2ct5d9ek293pqthvwfzadd7" +
		"jejes8q9lhc4rvjxd022zv5l44g6qah82ru5rdpnpjkp" +
		"pqvjx204vgdzgsqpvcp4mldl3plscny0rt707gvpdh6nd" +
		"ydfacz43euzqhrurageg3n7kafgsek6gz3e9w52parv8g" +
		"s2hlxzk95tzeswywffxlkeyhml0hh46kndmwf4m6xma3t" +
		"kq2lu04qz3slje2rfthc89vss"

	_, err := DecodeOfferString(
		lnrStr, farFutureNow(), bitcoinMainnetGenesisHash,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected HRP")
}

// TestEncodeOfferString tests round-trip via the convenience API.
func TestEncodeOfferString(t *testing.T) {
	t.Parallel()

	// Decode an offer, then re-encode and decode again.
	vec := findTestVector(t, "Minimal bolt12 offer")

	offer, err := DecodeOfferString(
		vec.Bolt12, farFutureNow(), bitcoinMainnetGenesisHash,
	)
	require.NoError(t, err)

	encoded, err := EncodeOfferString(offer)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	offer2, err := DecodeOfferString(
		encoded, farFutureNow(), bitcoinMainnetGenesisHash,
	)
	require.NoError(t, err)

	var id1, id2 *btcec.PublicKey
	offer.OfferIssuerID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType22, *btcec.PublicKey]) {
			id1 = r.Val
		},
	)
	offer2.OfferIssuerID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType22, *btcec.PublicKey]) {
			id2 = r.Val
		},
	)
	require.Equal(t,
		hex.EncodeToString(id1.SerializeCompressed()),
		hex.EncodeToString(id2.SerializeCompressed()))
}

// TestEncodeInvoiceRequestStringRoundTrip drives the lnr write path
// end to end: encode a valid invoice request, decode the resulting
// bech32 string, assert the recovered struct matches. Without this
// the encoder is exercised only transitively through
// signature_test.go and any mistake in HRP routing or TLV ordering
// would surface only at the network boundary.
func TestEncodeInvoiceRequestStringRoundTrip(t *testing.T) {
	t.Parallel()

	ir := validInvoiceRequest(t)

	encoded, err := EncodeInvoiceRequestString(ir)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	decoded, err := DecodeInvoiceRequestString(
		encoded, bitcoinMainnetGenesisHash,
	)
	require.NoError(t, err)

	// Round-trip preserves the TLV stream byte-for-byte. Re-encoding
	// the decoded request must yield the same TLV bytes the original
	// encode produced.
	originalBytes, err := ir.Encode()
	require.NoError(t, err)
	decodedBytes, err := decoded.Encode()
	require.NoError(t, err)
	require.Equal(t, originalBytes, decodedBytes)
}

// TestEncodeInvoiceRequestStringRejectsInvalid asserts the validate-
// first gate fires before bytes are emitted. An invalid request
// (here, missing payer ID) must surface ErrMissingPayerID rather
// than silently emit a structurally invalid lnr string that the
// receiver would reject at the next hop.
func TestEncodeInvoiceRequestStringRejectsInvalid(t *testing.T) {
	t.Parallel()

	ir := validInvoiceRequest(t)
	ir.InvreqPayerID = tlv.OptionalRecordT[
		tlv.TlvType88, *btcec.PublicKey,
	]{}

	encoded, err := EncodeInvoiceRequestString(ir)
	require.ErrorIs(t, err, ErrMissingPayerID)
	require.Empty(t, encoded)
}

// TestEncodeInvoiceStringRoundTrip is the lni analogue: encode a
// valid invoice, decode it back, assert the TLV stream survives
// byte-for-byte. The lni write path is the on-wire format for
// receiver-to-sender invoice replies; any drift in HRP routing or
// TLV ordering would silently corrupt every offer settlement. The
// invoice must be signed before encoding because DecodeInvoiceString
// runs the reader requirements, which reject unsigned invoices.
func TestEncodeInvoiceStringRoundTrip(t *testing.T) {
	t.Parallel()

	priv, pub := bobKey()
	inv := validInvoice(t)
	inv.InvoiceNodeID = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType176](pub),
	)
	_, err := inv.Encode()
	require.NoError(t, err)

	sig, err := SignInvoice(inv, priv)
	require.NoError(t, err)
	inv.Signature = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType240, [64]byte](sig),
	)

	encoded, err := EncodeInvoiceString(inv)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	decoded, err := DecodeInvoiceString(
		encoded, time.Unix(1234567890+1, 0),
		bitcoinMainnetGenesisHash,
	)
	require.NoError(t, err)

	// Round-trip preserves the TLV stream byte-for-byte.
	originalBytes, err := inv.Encode()
	require.NoError(t, err)
	decodedBytes, err := decoded.Encode()
	require.NoError(t, err)
	require.Equal(t, originalBytes, decodedBytes)
}

// TestEncodeInvoiceStringRejectsInvalid asserts EncodeInvoiceString
// refuses to emit bytes for an invoice that fails the writer
// requirements. Strips the payment hash from a signed baseline so the
// writer-requirements branch is exercised independently of the
// wire-string signature gate.
func TestEncodeInvoiceStringRejectsInvalid(t *testing.T) {
	t.Parallel()

	inv := validInvoice(t)
	inv.Signature = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType240, [64]byte]([64]byte{}),
	)
	inv.InvoicePaymentHash = tlv.OptionalRecordT[
		tlv.TlvType168, [32]byte,
	]{}

	encoded, err := EncodeInvoiceString(inv)
	require.ErrorIs(t, err, ErrMissingPaymentHash)
	require.Empty(t, encoded)
}

// TestDecodeInvoiceStringWrongHRP asserts the lni decoder rejects an
// lno (offer) string instead of silently parsing it as an invoice.
// Without HRP discrimination a sender could attempt to settle against
// an offer's TLV stream.
func TestDecodeInvoiceStringWrongHRP(t *testing.T) {
	t.Parallel()

	vec := findTestVector(t, "Minimal bolt12 offer")

	inv, err := DecodeInvoiceString(
		vec.Bolt12, farFutureNow(), bitcoinMainnetGenesisHash,
	)
	require.Error(t, err)
	require.Nil(t, inv)
	require.Contains(t, err.Error(), "expected HRP")
}

// TestDecodeInvoiceStringRejectsInvalid feeds the lni decoder a
// malformed bech32 string and asserts the bech32 layer's error is
// surfaced rather than silently returning a partial Invoice.
func TestDecodeInvoiceStringRejectsInvalid(t *testing.T) {
	t.Parallel()

	inv, err := DecodeInvoiceString(
		"not a bech32 string", farFutureNow(),
		bitcoinMainnetGenesisHash,
	)
	require.Error(t, err)
	require.Nil(t, inv)
}

// TestDecodeInvoiceRequestString tests the invoice request convenience
// function.
func TestDecodeInvoiceRequestString(t *testing.T) {
	t.Parallel()

	lnrStr := "lnr1qqyqqqqqqqqqqqqqqcp4256ypqqkgzshgysy6ct" +
		"5dpjk6ct5d93kzmpq23ex2ct5d9ek293pqthvwfzadd7" +
		"jejes8q9lhc4rvjxd022zv5l44g6qah82ru5rdpnpjkp" +
		"pqvjx204vgdzgsqpvcp4mldl3plscny0rt707gvpdh6nd" +
		"ydfacz43euzqhrurageg3n7kafgsek6gz3e9w52parv8g" +
		"s2hlxzk95tzeswywffxlkeyhml0hh46kndmwf4m6xma3t" +
		"kq2lu04qz3slje2rfthc89vss"

	ir, err := DecodeInvoiceRequestString(lnrStr, bitcoinMainnetGenesisHash)
	require.NoError(t, err)

	var desc []byte
	ir.OfferDescription.WhenSome(
		func(r tlv.RecordT[tlv.TlvType10, tlv.Blob]) {
			desc = r.Val
		},
	)
	require.Equal(t, "A Mathematical Treatise", string(desc))
}

// TestEncodeInvoiceRequestStringRequiresSignature pins H2 on the lnr
// path: the wire-string layer must refuse to emit an unsigned invoice
// request. (*InvoiceRequest).Encode is permitted to run unsigned so a
// caller can build → encode → sign → encode without first synthesising
// a signature, but the bech32 wrapper is the boundary at which the
// signature becomes mandatory.
func TestEncodeInvoiceRequestStringRequiresSignature(t *testing.T) {
	t.Parallel()

	ir := validInvoiceRequest(t)
	ir.Signature = tlv.OptionalRecordT[tlv.TlvType240, [64]byte]{}

	encoded, err := EncodeInvoiceRequestString(ir)
	require.ErrorIs(t, err, ErrMissingSignature)
	require.Empty(t, encoded)
}

// TestDecodeInvoiceStringEnforcesExpiry pins the bech32-wrapper
// expiry gate: an expired invoice must surface ErrInvoiceExpired
// before the caller can dereference the decoded struct, even though
// the underlying ValidateInvoiceRead succeeds. Without this gate
// Layer 2 callers who forget to invoke ValidateInvoiceExpiry would
// silently pay an expired invoice.
func TestDecodeInvoiceStringEnforcesExpiry(t *testing.T) {
	t.Parallel()

	priv, pub := bobKey()
	inv := validInvoice(t)
	inv.InvoiceNodeID = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType176](pub),
	)
	_, err := inv.Encode()
	require.NoError(t, err)
	sig, err := SignInvoice(inv, priv)
	require.NoError(t, err)
	inv.Signature = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType240, [64]byte](sig),
	)

	encoded, err := EncodeInvoiceString(inv)
	require.NoError(t, err)

	// validInvoice's InvoiceCreatedAt is 1234567890; default expiry
	// 7200s. Pick "now" past the window.
	expired := time.Unix(1234567890+8000, 0)
	decoded, err := DecodeInvoiceString(
		encoded, expired, bitcoinMainnetGenesisHash,
	)
	require.ErrorIs(t, err, ErrInvoiceExpired)
	require.Nil(t, decoded)
}

// TestEncodeInvoiceStringRequiresSignature pins H2 on the lni path:
// EncodeInvoiceString must refuse to emit an unsigned invoice for the
// same reasons as the lnr case. A receiver-side bug that omitted the
// signing step would otherwise quietly produce strings every conformant
// peer rejects.
func TestEncodeInvoiceStringRequiresSignature(t *testing.T) {
	t.Parallel()

	priv, pub := bobKey()
	inv := validInvoice(t)
	inv.InvoiceNodeID = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType176](pub),
	)

	// Run the pre-sign Encode so the writer-side validate succeeds,
	// then deliberately skip Sign and clear the signature field.
	// This mirrors the failure mode where a caller forgets the
	// signing step in the build → encode → sign → encode pipeline.
	_, err := inv.Encode()
	require.NoError(t, err)
	_ = priv

	require.False(t, inv.Signature.IsSome())

	encoded, err := EncodeInvoiceString(inv)
	require.ErrorIs(t, err, ErrMissingSignature)
	require.Empty(t, encoded)
}
