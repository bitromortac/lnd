package bolt12handler

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// TestGenerateInvoice_HappyPath verifies that a valid invoice is generated from
// a well-formed invoice request.
func TestGenerateInvoice_HappyPath(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	offer := testOffer(t, nodeKey, 10000)

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	ir := testInvoiceRequest(t, offer, payerKey, 10000)

	result, err := GenerateInvoice(ir, NewPrivKeySigner(nodeKey), nil, [32]byte{})
	require.NoError(t, err)

	// Verify the result has all required fields.
	require.NotEmpty(t, result.Encoded)
	require.NotEqual(t, [32]byte{}, result.Preimage)
	require.NotEqual(t, [32]byte{}, result.PaymentHash)
	require.NotEqual(t, [32]byte{}, result.PathID)

	// Verify the preimage hashes to the payment hash.
	require.Equal(t, result.Preimage.Hash(), result.PaymentHash)

	// Verify the encoded string decodes and verifies.
	decoded, err := bolt12.DecodeInvoiceString(result.Encoded, time.Now(), testChainHash())
	require.NoError(t, err)
	require.NoError(t, bolt12.VerifyInvoice(decoded))

	// Verify the invoice amount matches.
	var amt uint64
	decoded.InvoiceAmount.WhenSome(
		func(r tlv.RecordT[tlv.TlvType170, bolt12.TUint64]) {
			amt = uint64(r.Val)
		},
	)
	require.Equal(t, uint64(10000), amt)

	// Verify invoice_node_id matches our node key.
	var nodeIDBytes []byte
	decoded.InvoiceNodeID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType176, *btcec.PublicKey]) {
			if r.Val != nil {
				nodeIDBytes = r.Val.SerializeCompressed()
			}
		},
	)
	require.Equal(t,
		nodeKey.PubKey().SerializeCompressed(),
		nodeIDBytes,
	)

	// Verify mirrored fields.
	var descBytes []byte
	decoded.OfferDescription.WhenSome(
		func(r tlv.RecordT[tlv.TlvType10, tlv.Blob]) {
			descBytes = r.Val
		},
	)
	require.Equal(t, "test offer", string(descBytes))

	// Verify the payer ID is mirrored.
	var payerID []byte
	decoded.InvreqPayerID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType88, *btcec.PublicKey]) {
			payerID = r.Val.SerializeCompressed()
		},
	)
	require.Equal(
		t, payerKey.PubKey().SerializeCompressed(), payerID,
	)
}

// TestGenerateInvoice_NoFixedAmount verifies invoice generation when the offer
// has no fixed amount and invreq_amount is provided.
func TestGenerateInvoice_NoFixedAmount(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	offer := testOffer(t, nodeKey, 0) // No fixed amount.

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	ir := testInvoiceRequest(t, offer, payerKey, 25000)

	result, err := GenerateInvoice(ir, NewPrivKeySigner(nodeKey), nil, [32]byte{})
	require.NoError(t, err)

	decoded, err := bolt12.DecodeInvoiceString(result.Encoded, time.Now(), testChainHash())
	require.NoError(t, err)
	require.NoError(t, bolt12.VerifyInvoice(decoded))

	var amt uint64
	decoded.InvoiceAmount.WhenSome(
		func(r tlv.RecordT[tlv.TlvType170, bolt12.TUint64]) {
			amt = uint64(r.Val)
		},
	)
	require.Equal(t, uint64(25000), amt)
}

// TestGenerateInvoice_WithQuantity verifies that invoice amount is computed
// correctly when quantity is present.
func TestGenerateInvoice_WithQuantity(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	offer := testOffer(t, nodeKey, 1000)
	offer.HasQuantityMax = true
	offer.QuantityMax = 10

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	// Request 5 items with no explicit invreq_amount — the expected amount
	// should be 5 * 1000 = 5000.
	ir := testInvoiceRequest(t, offer, payerKey, 0)
	qty := bolt12.TUint64(5)
	ir.InvreqQuantity = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType86, bolt12.TUint64]{
			Val: qty,
		},
	)

	result, err := GenerateInvoice(ir, NewPrivKeySigner(nodeKey), nil, [32]byte{})
	require.NoError(t, err)

	decoded, err := bolt12.DecodeInvoiceString(result.Encoded, time.Now(), testChainHash())
	require.NoError(t, err)

	var amt uint64
	decoded.InvoiceAmount.WhenSome(
		func(r tlv.RecordT[tlv.TlvType170, bolt12.TUint64]) {
			amt = uint64(r.Val)
		},
	)
	require.Equal(t, uint64(5000), amt)
}

// TestGenerateInvoice_BlindedPath verifies that the generated invoice includes
// a blinded payment path with a path_id.
func TestGenerateInvoice_BlindedPath(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	offer := testOffer(t, nodeKey, 10000)

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	ir := testInvoiceRequest(t, offer, payerKey, 10000)

	result, err := GenerateInvoice(ir, NewPrivKeySigner(nodeKey), nil, [32]byte{})
	require.NoError(t, err)

	decoded, err := bolt12.DecodeInvoiceString(result.Encoded, time.Now(), testChainHash())
	require.NoError(t, err)

	// Verify invoice_paths is present with one path.
	hasPaths := false
	decoded.InvoicePaths.WhenSome(
		func(r tlv.RecordT[tlv.TlvType160, lnwire.BlindedPaths]) {

			hasPaths = true
			require.Len(t, r.Val.Paths, 1)

			path := r.Val.Paths[0]
			require.Len(t, path.Hops, 1)

			// The single hop's encrypted data contains the
			// path_id in encrypted form.
			require.NotEmpty(t, path.Hops[0].EncryptedData)
		},
	)
	require.True(t, hasPaths)

	// Verify invoice_blindedpay is present.
	hasPayInfo := false
	decoded.InvoiceBlindedPay.WhenSome(
		func(r tlv.RecordT[tlv.TlvType162, bolt12.BlindedPayInfos]) {

			hasPayInfo = true
			require.Len(t, r.Val.Infos, 1)
		},
	)
	require.True(t, hasPayInfo)
}

// TestGenerateInvoice_UniquePreimages verifies that each invocation generates a
// unique preimage and path_id.
func TestGenerateInvoice_UniquePreimages(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	offer := testOffer(t, nodeKey, 10000)

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	ir := testInvoiceRequest(t, offer, payerKey, 10000)

	signer := NewPrivKeySigner(nodeKey)

	r1, err := GenerateInvoice(ir, signer, nil, [32]byte{})
	require.NoError(t, err)

	r2, err := GenerateInvoice(ir, signer, nil, [32]byte{})
	require.NoError(t, err)

	require.NotEqual(t, r1.Preimage, r2.Preimage)
	require.NotEqual(t, r1.PaymentHash, r2.PaymentHash)
	require.NotEqual(t, r1.PathID, r2.PathID)
}
