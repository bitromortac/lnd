package bolt12handler

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/offers"
	"github.com/stretchr/testify/require"
)

// newTestReconstructor creates a Reconstructor with a PrivKeySigner and a mock
// offer store pre-loaded with one offer. Returns the reconstructor, the node
// private key, and the offer.
func newTestReconstructor(t *testing.T) (*Reconstructor, *btcec.PrivateKey,
	*offers.Offer) {

	t.Helper()

	nodeKey := testKey(t)
	signer := NewPrivKeySigner(nodeKey)
	store := newMockOfferStore()

	offer := &offers.Offer{
		ID:          1,
		OfferID:     [32]byte{0xAA, 0xBB, 0xCC},
		Description: "test offer",
	}
	store.offers[offer.OfferID] = offer

	r := NewReconstructor(signer, store)

	return r, nodeKey, offer
}

// buildTestEnvelope creates a signed envelope with the given parameters, using
// the node key for signing.
func buildTestEnvelope(t *testing.T, nodeKey *btcec.PrivateKey,
	offerIDHash [32]byte, preimage [32]byte, createdAt uint64,
	amount uint64) ([]byte, lntypes.Hash) {

	t.Helper()

	payerPub := testCompressedPubKey(t)

	data := &InvoiceEnvelopeData{
		Preimage:  preimage,
		PayerID:   payerPub,
		CreatedAt: createdAt,
		Amount:    amount,
	}

	signed, err := SignEnvelope(nodeKey, offerIDHash, data)
	require.NoError(t, err)

	paymentHash := sha256.Sum256(preimage[:])

	return EncodeSignedEnvelope(signed), lntypes.Hash(paymentHash)
}

// TestReconstruct_Valid verifies that a valid envelope reconstructs a correct
// invoice.
func TestReconstruct_Valid(t *testing.T) {
	t.Parallel()

	r, nodeKey, offer := newTestReconstructor(t)

	preimage := [32]byte{1, 2, 3, 4, 5}
	createdAt := uint64(time.Now().Unix())
	amount := uint64(50000)

	envBytes, paymentHash := buildTestEnvelope(
		t, nodeKey, offer.OfferID, preimage, createdAt, amount,
	)

	pathID := chainhash.Hash{0xDE, 0xAD}

	inv, err := r.ReconstructInvoice(
		context.Background(), envBytes, pathID, paymentHash,
	)
	require.NoError(t, err)

	require.True(t, inv.IsBolt12)
	require.Equal(t, offer.ID, *inv.OfferID)
	require.Equal(t, offer.OfferID[:], inv.OfferIDHash)
	require.Equal(t, [32]byte(pathID), inv.Terms.PaymentAddr)
	require.Equal(t, lntypes.Preimage(preimage),
		*inv.Terms.PaymentPreimage)
	require.Equal(t, amount, uint64(inv.Terms.Value))
	require.Equal(t,
		nodeKey.PubKey().SerializeCompressed(),
		inv.InvoiceNodeID,
	)
}

// TestReconstruct_BadSignature verifies that a tampered signature fails
// reconstruction.
func TestReconstruct_BadSignature(t *testing.T) {
	t.Parallel()

	r, nodeKey, offer := newTestReconstructor(t)

	preimage := [32]byte{10, 20, 30}
	createdAt := uint64(time.Now().Unix())
	envBytes, paymentHash := buildTestEnvelope(
		t, nodeKey, offer.OfferID, preimage, createdAt, 1000,
	)

	// Flip a signature bit (byte 5 in the wire format is inside the
	// 64-byte signature).
	envBytes[5] ^= 0x01

	pathID := chainhash.Hash{0x01}

	_, err := r.ReconstructInvoice(
		context.Background(), envBytes, pathID, paymentHash,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "verify envelope")
}

// TestReconstruct_HashMismatch verifies that reconstruction fails when the
// payment hash does not match sha256(preimage).
func TestReconstruct_HashMismatch(t *testing.T) {
	t.Parallel()

	r, nodeKey, offer := newTestReconstructor(t)

	preimage := [32]byte{40, 50, 60}
	createdAt := uint64(time.Now().Unix())
	envBytes, _ := buildTestEnvelope(
		t, nodeKey, offer.OfferID, preimage, createdAt, 2000,
	)

	// Use a wrong payment hash.
	wrongHash := lntypes.Hash{0xFF, 0xFF}
	pathID := chainhash.Hash{0x02}

	_, err := r.ReconstructInvoice(
		context.Background(), envBytes, pathID, wrongHash,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "preimage hash mismatch")
}

// TestReconstruct_Expired verifies that reconstruction fails for envelopes
// older than createdAt + defaultRelativeExpiry.
func TestReconstruct_Expired(t *testing.T) {
	t.Parallel()

	r, nodeKey, offer := newTestReconstructor(t)

	preimage := [32]byte{70, 80, 90}

	// Set createdAt to 3 hours ago — well past the 2h expiry.
	createdAt := uint64(time.Now().Unix()) - 3*3600

	envBytes, paymentHash := buildTestEnvelope(
		t, nodeKey, offer.OfferID, preimage, createdAt, 3000,
	)

	pathID := chainhash.Hash{0x03}

	_, err := r.ReconstructInvoice(
		context.Background(), envBytes, pathID, paymentHash,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "envelope expired")
}

// TestReconstruct_UnknownOffer verifies that reconstruction fails when the
// offer ID in the envelope does not match any stored offer.
func TestReconstruct_UnknownOffer(t *testing.T) {
	t.Parallel()

	r, nodeKey, _ := newTestReconstructor(t)

	preimage := [32]byte{11, 22, 33}
	createdAt := uint64(time.Now().Unix())

	// Sign with an offer ID that doesn't exist in the store.
	unknownOfferID := [32]byte{0xFF, 0xFE, 0xFD}
	envBytes, paymentHash := buildTestEnvelope(
		t, nodeKey, unknownOfferID, preimage, createdAt, 4000,
	)

	pathID := chainhash.Hash{0x04}

	_, err := r.ReconstructInvoice(
		context.Background(), envBytes, pathID, paymentHash,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "lookup offer")
}
