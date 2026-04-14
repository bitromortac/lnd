package invoicesrpc

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/invoices"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/stretchr/testify/require"
)

// testPreimage is a deterministic preimage used across tests.
var testPreimage = lntypes.Preimage{
	0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
	0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
	0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
	0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
}

// TestDecodePayReq_Bolt12 verifies that decodePayReq returns a valid
// zpay32.Invoice with the correct payment hash for a BOLT 12 invoice
// instead of attempting to zpay32-decode the lni1... payment request.
func TestDecodePayReq_Bolt12(t *testing.T) {
	t.Parallel()

	preimage := testPreimage
	inv := &invoices.Invoice{
		PaymentRequest: []byte("lni1...fake-bolt12-invoice"),
		IsBolt12:       true,
		Terms: invoices.ContractTerm{
			PaymentPreimage: &preimage,
		},
	}

	decoded, err := decodePayReq(inv, &chaincfg.MainNetParams)
	require.NoError(t, err)
	require.NotNil(t, decoded.PaymentHash)

	expectedHash := preimage.Hash()
	require.Equal(t, expectedHash[:], decoded.PaymentHash[:])
}

// TestDecodePayReq_Bolt12_NilPreimage verifies that a BOLT 12 invoice
// without a preimage (e.g. a future hold invoice) returns an empty
// zpay32.Invoice without error.
func TestDecodePayReq_Bolt12_NilPreimage(t *testing.T) {
	t.Parallel()

	inv := &invoices.Invoice{
		PaymentRequest: []byte("lni1...fake-bolt12-invoice"),
		IsBolt12:       true,
		Terms:          invoices.ContractTerm{},
	}

	decoded, err := decodePayReq(inv, &chaincfg.MainNetParams)
	require.NoError(t, err)
	require.Nil(t, decoded.PaymentHash)
}

// TestDecodePayReq_Keysend verifies the existing keysend path (empty
// payment request) is unchanged.
func TestDecodePayReq_Keysend(t *testing.T) {
	t.Parallel()

	preimage := testPreimage
	inv := &invoices.Invoice{
		PaymentRequest: nil,
		Terms: invoices.ContractTerm{
			PaymentPreimage: &preimage,
		},
	}

	decoded, err := decodePayReq(inv, &chaincfg.MainNetParams)
	require.NoError(t, err)
	require.NotNil(t, decoded.PaymentHash)

	expectedHash := preimage.Hash()
	require.Equal(t, expectedHash[:], decoded.PaymentHash[:])
}

// TestCreateRPCInvoice_Bolt12 verifies that CreateRPCInvoice succeeds
// for a BOLT 12 invoice and populates the core fields correctly.
func TestCreateRPCInvoice_Bolt12(t *testing.T) {
	t.Parallel()

	preimage := testPreimage
	paymentAddr := [32]byte{0xaa, 0xbb}

	offerHash := [32]byte{0xde, 0xad}
	nodeID := []byte{0x02, 0x01, 0x02}
	payerID := []byte{0x03, 0x04, 0x05}

	inv := &invoices.Invoice{
		Memo:           []byte("bolt12 test"),
		PaymentRequest: []byte("lni1...fake-bolt12-invoice"),
		CreationDate:   time.Unix(1700000000, 0),
		IsBolt12:       true,
		OfferIDHash:    offerHash[:],
		InvoiceNodeID:  nodeID,
		InvreqPayerID:  payerID,
		Terms: invoices.ContractTerm{
			PaymentPreimage: &preimage,
			PaymentAddr:     paymentAddr,
			Value:           lnwire.MilliSatoshi(100_000),
			Expiry:          7200 * time.Second,
			Features:        lnwire.EmptyFeatureVector(),
		},
		State: invoices.ContractOpen,
	}

	rpcInv, err := CreateRPCInvoice(inv, &chaincfg.MainNetParams)
	require.NoError(t, err)

	// Core fields.
	require.Equal(t, "bolt12 test", rpcInv.Memo)
	require.Equal(t, int64(100_000), rpcInv.ValueMsat)
	require.Equal(t, int64(7200), rpcInv.Expiry)
	require.Equal(t,
		"lni1...fake-bolt12-invoice", rpcInv.PaymentRequest,
	)

	expectedHash := preimage.Hash()
	require.Equal(t, expectedHash[:], rpcInv.RHash)
	require.Equal(t, preimage[:], rpcInv.RPreimage)

	// BOLT 12 fields.
	require.True(t, rpcInv.IsBolt12)
	require.NotNil(t, rpcInv.Bolt12Detail)
	require.Equal(t, offerHash[:], rpcInv.Bolt12Detail.OfferId)
	require.Equal(t, nodeID, rpcInv.Bolt12Detail.InvoiceNodeId)
	require.Equal(t, payerID, rpcInv.Bolt12Detail.InvreqPayerId)
}

// TestCreateRPCInvoice_Bolt11_NoBolt12Detail verifies that BOLT 11
// invoices have is_bolt12=false and no bolt12_detail.
func TestCreateRPCInvoice_Bolt11_NoBolt12Detail(t *testing.T) {
	t.Parallel()

	preimage := testPreimage
	paymentAddr := [32]byte{0xcc}

	inv := &invoices.Invoice{
		Memo:         []byte("bolt11 test"),
		CreationDate: time.Unix(1700000000, 0),
		Terms: invoices.ContractTerm{
			PaymentPreimage: &preimage,
			PaymentAddr:     paymentAddr,
			Value:           lnwire.MilliSatoshi(50_000),
			Expiry:          3600 * time.Second,
			Features:        lnwire.EmptyFeatureVector(),
		},
		State: invoices.ContractOpen,
	}

	rpcInv, err := CreateRPCInvoice(inv, &chaincfg.MainNetParams)
	require.NoError(t, err)

	require.False(t, rpcInv.IsBolt12)
	require.Nil(t, rpcInv.Bolt12Detail)
}
