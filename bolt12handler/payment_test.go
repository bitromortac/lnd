package bolt12handler

import (
	"crypto/sha256"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// TestBolt12InvoiceToBlindedPathSet_SingleHop verifies that a single-hop
// blinded path from a receiver-generated invoice converts correctly.
func TestBolt12InvoiceToBlindedPathSet_SingleHop(t *testing.T) {
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

	// Round-trip the invoice through encode/decode.
	invBytes, err := result.Invoice.Encode()
	require.NoError(t, err)

	inv, err := bolt12.DecodeInvoice(invBytes)
	require.NoError(t, err)

	pathSet, err := Bolt12InvoiceToBlindedPathSet(inv, nil)
	require.NoError(t, err)
	require.NotNil(t, pathSet)

	// The target pubkey should be derivable.
	targetPub := pathSet.TargetPubKey()
	require.NotNil(t, targetPub)
}

// TestBolt12InvoiceToBlindedPathSet_MissingPaths verifies that an invoice
// without blinded paths returns an error.
func TestBolt12InvoiceToBlindedPathSet_MissingPaths(t *testing.T) {
	t.Parallel()

	inv := &bolt12.Invoice{}

	_, err := Bolt12InvoiceToBlindedPathSet(inv, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no usable blinded paths")
}

// TestBolt12InvoiceToBlindedPathSet_CountMismatch verifies that mismatched
// path and pay info counts return an error.
func TestBolt12InvoiceToBlindedPathSet_CountMismatch(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	nodePub := nodeKey.PubKey()
	introPub, err := lnwire.NewPubkeyIntro(nodePub)
	require.NoError(t, err)

	inv := &bolt12.Invoice{}
	inv.InvoicePaths = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType160, lnwire.BlindedPaths]{
			Val: lnwire.BlindedPaths{
				Paths: []lnwire.BlindedPath{
					{
						IntroductionNode: introPub,
						BlindingPoint:    nodePub,
						Hops: []lnwire.BlindedHop{{
							BlindedNodeID: nodePub,
							EncryptedData: []byte{0},
						}},
					},
				},
			},
		},
	)
	// Set pay info with zero entries — mismatch.
	inv.InvoiceBlindedPay = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType162, bolt12.BlindedPayInfos]{
			Val: bolt12.BlindedPayInfos{
				Infos: []bolt12.BlindedPayInfo{},
			},
		},
	)

	// A path/pay-info count mismatch leaves UsablePaths with nothing to
	// pair, so it surfaces as the unified "no usable blinded paths" error.
	_, err = Bolt12InvoiceToBlindedPathSet(inv, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no usable blinded paths")
}

// TestBolt12InvoiceToBlindedPathSet_NilPubkey verifies that a nil
// introduction node pubkey returns an error rather than panicking.
func TestBolt12InvoiceToBlindedPathSet_NilPubkey(t *testing.T) {
	t.Parallel()

	badIntro := lnwire.PubkeyIntro{Pubkey: nil}

	inv := &bolt12.Invoice{}
	inv.InvoicePaths = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType160, lnwire.BlindedPaths]{
			Val: lnwire.BlindedPaths{
				Paths: []lnwire.BlindedPath{
					{
						IntroductionNode: badIntro,
						BlindingPoint:    nil,
						Hops: []lnwire.BlindedHop{{
							BlindedNodeID: nil,
							EncryptedData: []byte{0},
						}},
					},
				},
			},
		},
	)
	inv.InvoiceBlindedPay = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType162, bolt12.BlindedPayInfos]{
			Val: bolt12.BlindedPayInfos{
				Infos: []bolt12.BlindedPayInfo{{
					HtlcMaximumMsat: 100000,
				}},
			},
		},
	)

	_, err := Bolt12InvoiceToBlindedPathSet(inv, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil pubkey")
}

// TestBuildLightningPayment_Valid verifies that a payment is correctly
// constructed from a valid invoice.
func TestBuildLightningPayment_Valid(t *testing.T) {
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

	invBytes, err := result.Invoice.Encode()
	require.NoError(t, err)

	inv, err := bolt12.DecodeInvoice(invBytes)
	require.NoError(t, err)

	pathSet, err := Bolt12InvoiceToBlindedPathSet(inv, nil)
	require.NoError(t, err)

	offerTLV, err := offer.Encode()
	require.NoError(t, err)
	offerIDHash := sha256.Sum256(offerTLV)

	payment, err := BuildLightningPayment(
		inv, pathSet, "lni1test", offerIDHash[:], 0, 30,
	)
	require.NoError(t, err)

	require.Equal(
		t, lnwire.MilliSatoshi(50000), payment.Amount,
	)
	require.NotNil(t, payment.BlindedPathSet)
	// Generated invoices advertise OPT_BASIC_MPP, so the sender uses
	// the default MPP shard limit.
	require.Equal(t, uint32(defaultMaxParts), payment.MaxParts)
	require.Equal(t, []byte("lni1test"), payment.PaymentRequest)
	require.Equal(t, offerIDHash[:], payment.OfferID)

	// Fee limit should be the default percentage (1% of 50000 =
	// 500, but minimum 1).
	require.Equal(
		t, lnwire.MilliSatoshi(500), payment.FeeLimit,
	)
}

// TestBuildLightningPayment_MissingHash verifies that a missing payment
// hash returns an error.
func TestBuildLightningPayment_MissingHash(t *testing.T) {
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

	invBytes, err := result.Invoice.Encode()
	require.NoError(t, err)

	inv, err := bolt12.DecodeInvoice(invBytes)
	require.NoError(t, err)

	// Clear the payment hash.
	inv.InvoicePaymentHash = tlv.OptionalRecordT[
		tlv.TlvType168, [32]byte,
	]{}

	pathSet, err := Bolt12InvoiceToBlindedPathSet(inv, nil)
	require.NoError(t, err)

	_, err = BuildLightningPayment(
		inv, pathSet, "lni1test", nil, 0, 30,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "payment hash")
}

// TestBuildLightningPayment_ExplicitFeeLimit verifies that an explicit fee
// limit overrides the default.
func TestBuildLightningPayment_ExplicitFeeLimit(t *testing.T) {
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

	invBytes, err := result.Invoice.Encode()
	require.NoError(t, err)

	inv, err := bolt12.DecodeInvoice(invBytes)
	require.NoError(t, err)

	pathSet, err := Bolt12InvoiceToBlindedPathSet(inv, nil)
	require.NoError(t, err)

	payment, err := BuildLightningPayment(
		inv, pathSet, "lni1test", nil, 2000, 30,
	)
	require.NoError(t, err)

	require.Equal(
		t, lnwire.MilliSatoshi(2000), payment.FeeLimit,
	)
}

// testKey returns a deterministic test key (shared helper defined in
// request_test.go, but we need it available here too). This is a
// compile-time check that the shared helper exists.
var _ = func(t *testing.T) *btcec.PrivateKey {
	return testKey(t)
}
