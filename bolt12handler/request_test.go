package bolt12handler

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// TestBuildInvoiceRequest_BasicOffer verifies that an invoice request is
// correctly constructed from an offer with a fixed amount.
func TestBuildInvoiceRequest_BasicOffer(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	offer := buildBolt12Offer(t, nodeKey, 50000)

	ir, payerKey, err := BuildInvoiceRequest(offer)
	require.NoError(t, err)
	require.NotNil(t, payerKey)

	// Verify offer fields are mirrored.
	var desc []byte
	ir.OfferDescription.WhenSome(
		func(r tlv.RecordT[tlv.TlvType10, tlv.Blob]) {
			desc = r.Val
		},
	)
	require.Equal(t, "test offer", string(desc))

	require.Equal(
		t, uint64(50000),
		getUint64Field(ir.OfferAmount),
	)

	// Verify payer fields are set.
	require.True(t, hasOptField(ir.InvreqPayerID))
	require.True(t, hasOptField(ir.InvreqMetadata))

	// Verify the payer ID matches the returned key.
	var payerID []byte
	ir.InvreqPayerID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType88, *btcec.PublicKey]) {
			payerID = r.Val.SerializeCompressed()
		},
	)
	require.Equal(
		t, payerKey.PubKey().SerializeCompressed(), payerID,
	)

	// Verify signature is present and valid.
	require.True(t, hasOptField(ir.Signature))

	// Re-encode and decode to verify round-trip, then check sig.
	irBytes, err := ir.Encode()
	require.NoError(t, err)

	decoded, err := bolt12.DecodeInvoiceRequest(irBytes)
	require.NoError(t, err)
	require.NoError(t, bolt12.VerifyInvoiceRequest(decoded))
}

// TestBuildInvoiceRequest_WithAmount verifies that invreq_amount is set when
// the offer has no fixed amount.
func TestBuildInvoiceRequest_WithAmount(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	offer := buildBolt12Offer(t, nodeKey, 0) // No fixed amount.

	ir, _, err := BuildInvoiceRequest(
		offer, WithAmount(25000),
	)
	require.NoError(t, err)

	require.Equal(
		t, uint64(25000),
		getUint64Field(ir.InvreqAmount),
	)
}

// TestBuildInvoiceRequest_WithQuantity verifies that invreq_quantity is set
// when provided.
func TestBuildInvoiceRequest_WithQuantity(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	offer := buildBolt12Offer(t, nodeKey, 1000)

	// invreq_quantity is only valid when the offer advertises
	// offer_quantity_max, so set one before requesting a quantity.
	offer.OfferQuantityMax = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType20, bolt12.TUint64]{Val: bolt12.TUint64(5)},
	)

	ir, _, err := BuildInvoiceRequest(
		offer, WithQuantity(3),
	)
	require.NoError(t, err)

	require.Equal(
		t, uint64(3),
		getUint64Field(ir.InvreqQuantity),
	)
}

// TestBuildInvoiceRequest_WithPayerNote verifies that invreq_payer_note is set
// when provided.
func TestBuildInvoiceRequest_WithPayerNote(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	offer := buildBolt12Offer(t, nodeKey, 1000)

	ir, _, err := BuildInvoiceRequest(
		offer, WithPayerNote("for coffee"),
	)
	require.NoError(t, err)

	var note []byte
	ir.InvreqPayerNote.WhenSome(
		func(r tlv.RecordT[tlv.TlvType89, tlv.Blob]) {
			note = r.Val
		},
	)
	require.Equal(t, "for coffee", string(note))
}

// TestBuildSingleHopReplyPath verifies the reply path has one hop and the
// introduction node matches the given pubkey.
func TestBuildSingleHopReplyPath(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)

	pathInfo, err := BuildSingleHopReplyPath(nodeKey.PubKey())
	require.NoError(t, err)
	require.NotNil(t, pathInfo)
	require.NotNil(t, pathInfo.Path)
	require.Len(t, pathInfo.Path.BlindedHops, 1)
	require.Equal(
		t, nodeKey.PubKey(),
		pathInfo.Path.IntroductionPoint,
	)
}

// TestValidateInvoiceReply_Valid verifies that a correctly generated invoice
// passes validation.
func TestValidateInvoiceReply_Valid(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	signer := NewPrivKeySigner(nodeKey)

	offer := buildBolt12Offer(t, nodeKey, 50000)

	ir, _, err := BuildInvoiceRequest(offer)
	require.NoError(t, err)

	// Re-encode to populate rawTLVs for the receiver.
	irBytes, err := ir.Encode()
	require.NoError(t, err)

	ir, err = bolt12.DecodeInvoiceRequest(irBytes)
	require.NoError(t, err)

	// Generate an invoice using the receiver-side logic.
	result, err := GenerateInvoice(ir, signer, nil, [32]byte{})
	require.NoError(t, err)

	// The invoice must survive round-trip decode.
	invBytes, err := result.Invoice.Encode()
	require.NoError(t, err)

	inv, err := bolt12.DecodeInvoice(invBytes)
	require.NoError(t, err)

	err = ValidateInvoiceReply(inv, ir, offer, testChainHash())
	require.NoError(t, err)
}

// TestValidateInvoiceReply_NodeIDMismatch verifies rejection when
// invoice_node_id doesn't match offer_issuer_id.
func TestValidateInvoiceReply_NodeIDMismatch(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	signer := NewPrivKeySigner(nodeKey)

	offer := buildBolt12Offer(t, nodeKey, 50000)

	ir, _, err := BuildInvoiceRequest(offer)
	require.NoError(t, err)

	irBytes, err := ir.Encode()
	require.NoError(t, err)

	ir, err = bolt12.DecodeInvoiceRequest(irBytes)
	require.NoError(t, err)

	result, err := GenerateInvoice(ir, signer, nil, [32]byte{})
	require.NoError(t, err)

	// Tamper: change the offer_issuer_id to a different key.
	otherKey := testKey2(t)
	otherOffer := buildBolt12Offer(t, otherKey, 50000)

	invBytes, err := result.Invoice.Encode()
	require.NoError(t, err)

	inv, err := bolt12.DecodeInvoice(invBytes)
	require.NoError(t, err)

	err = ValidateInvoiceReply(inv, ir, otherOffer, testChainHash())
	require.Error(t, err)
	require.Contains(t, err.Error(), "invoice_node_id")
}

// buildBolt12Offer creates a bolt12.Offer for testing.
func buildBolt12Offer(t *testing.T, key *btcec.PrivateKey,
	amountMsat uint64) *bolt12.Offer {

	t.Helper()

	offer := &bolt12.Offer{}

	offer.OfferIssuerID = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType22, *btcec.PublicKey]{
			Val: key.PubKey(),
		},
	)
	offer.OfferDescription = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType10, tlv.Blob]{
			Val: []byte("test offer"),
		},
	)

	if amountMsat > 0 {
		amt := bolt12.TUint64(amountMsat)
		offer.OfferAmount = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType8, bolt12.TUint64]{
				Val: amt,
			},
		)
	}

	return offer
}

// testKey2 returns a second deterministic test key distinct from testKey.
func testKey2(t *testing.T) *btcec.PrivateKey {
	t.Helper()

	var seed [32]byte
	for i := range seed {
		seed[i] = byte(i + 100)
	}

	key, _ := btcec.PrivKeyFromBytes(seed[:])

	return key
}
