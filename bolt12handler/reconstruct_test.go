package bolt12handler

import (
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/invoices"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/offers"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// newTestReconstructor creates a Reconstructor with a privKeySigner and a mock
// offer store pre-loaded with one offer. Returns the reconstructor, the node
// private key, and the offer.
func newTestReconstructor(t *testing.T) (*Reconstructor, *btcec.PrivateKey,
	*offers.Offer) {

	t.Helper()

	nodeKey := testKey(t)
	store := newMockOfferStore()

	offer := &offers.Offer{
		ID:   1,
		Hash: [32]byte{0xAA, 0xBB, 0xCC},
	}
	store.offers[offer.Hash] = offer

	r := NewReconstructor(
		nodeKey.PubKey(), store, testChainHash(), testFinalCLTVDelta,
	)

	return r, nodeKey, offer
}

// buildTestEnvelope creates a signed envelope with the given parameters, using
// the node key for signing.
func buildTestEnvelope(t *testing.T, nodeKey *btcec.PrivateKey,
	offerHash [32]byte, preimage [32]byte, createdAt uint64,
	amount uint64) ([]byte, lntypes.Hash) {

	t.Helper()

	payerPub := testCompressedPubKey(t)

	data := &InvoiceEnvelopeData{
		Preimage:  preimage,
		PayerID:   payerPub,
		CreatedAt: createdAt,
		Amount:    amount,
	}

	signed, err := SignEnvelope(NewPrivKeySigner(nodeKey), offerHash, data)
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
		t, nodeKey, offer.Hash, preimage, createdAt, amount,
	)

	pathID := chainhash.Hash{0xDE, 0xAD}

	inv, err := r.ReconstructInvoice(
		t.Context(), envBytes, pathID, paymentHash,
	)
	require.NoError(t, err)

	require.True(t, inv.IsBolt12)
	require.Equal(t, fn.Some(offer.ID), inv.OfferID)
	require.Equal(t, offer.Hash[:], inv.OfferHash)
	require.Equal(t, [32]byte(pathID), inv.Terms.PaymentAddr)
	require.Equal(t, lntypes.Preimage(preimage),
		*inv.Terms.PaymentPreimage)
	require.Equal(t, amount, uint64(inv.Terms.Value))
	require.EqualValues(t, testFinalCLTVDelta, inv.Terms.FinalCltvDelta)
}

// TestReconstruct_Quantity verifies that the envelope quantity is restored on
// the reconstructed invoice.
func TestReconstruct_Quantity(t *testing.T) {
	t.Parallel()

	r, nodeKey, offer := newTestReconstructor(t)

	preimage := [32]byte{7, 7, 7}
	data := &InvoiceEnvelopeData{
		Preimage:  preimage,
		PayerID:   testCompressedPubKey(t),
		CreatedAt: uint64(time.Now().Unix()),
		Amount:    30000,
		Quantity:  3,
	}
	signed, err := SignEnvelope(
		NewPrivKeySigner(nodeKey), offer.Hash, data,
	)
	require.NoError(t, err)

	inv, err := r.ReconstructInvoice(
		t.Context(), EncodeSignedEnvelope(signed),
		chainhash.Hash{0xBE, 0xEF}, sha256.Sum256(preimage[:]),
	)
	require.NoError(t, err)
	require.Equal(t, uint64(3), inv.InvreqQuantity)
}

// TestReconstruct_BadSignature verifies that a tampered signature fails
// reconstruction.
func TestReconstruct_BadSignature(t *testing.T) {
	t.Parallel()

	r, nodeKey, offer := newTestReconstructor(t)

	preimage := [32]byte{10, 20, 30}
	createdAt := uint64(time.Now().Unix())
	envBytes, paymentHash := buildTestEnvelope(
		t, nodeKey, offer.Hash, preimage, createdAt, 1000,
	)

	// Flip a signature bit (byte 5 in the wire format is inside the
	// 64-byte signature).
	envBytes[5] ^= 0x01

	pathID := chainhash.Hash{0x01}

	_, err := r.ReconstructInvoice(
		t.Context(), envBytes, pathID, paymentHash,
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
		t, nodeKey, offer.Hash, preimage, createdAt, 2000,
	)

	// Use a wrong payment hash.
	wrongHash := lntypes.Hash{0xFF, 0xFF}
	pathID := chainhash.Hash{0x02}

	_, err := r.ReconstructInvoice(
		t.Context(), envBytes, pathID, wrongHash,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "preimage hash mismatch")
}

// TestReconstruct_Expired verifies that reconstruction fails for envelopes
// older than createdAt + bolt12.DefaultInvoiceRelativeExpiry.
func TestReconstruct_Expired(t *testing.T) {
	t.Parallel()

	r, nodeKey, offer := newTestReconstructor(t)

	preimage := [32]byte{70, 80, 90}

	// Set createdAt to 3 hours ago. This is well past the 2h expiry.
	createdAt := uint64(time.Now().Unix()) - 3*3600

	envBytes, paymentHash := buildTestEnvelope(
		t, nodeKey, offer.Hash, preimage, createdAt, 3000,
	)

	pathID := chainhash.Hash{0x03}

	_, err := r.ReconstructInvoice(
		t.Context(), envBytes, pathID, paymentHash,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "envelope expired")
}

// TestReconstruct_UnknownOffer verifies that reconstruction fails when the
// offer hash in the envelope does not match any stored offer.
func TestReconstruct_UnknownOffer(t *testing.T) {
	t.Parallel()

	r, nodeKey, _ := newTestReconstructor(t)

	preimage := [32]byte{11, 22, 33}
	createdAt := uint64(time.Now().Unix())

	// Sign with an offer hash that doesn't exist in the store.
	unknownOfferHash := [32]byte{0xFF, 0xFE, 0xFD}
	envBytes, paymentHash := buildTestEnvelope(
		t, nodeKey, unknownOfferHash, preimage, createdAt, 4000,
	)

	pathID := chainhash.Hash{0x04}

	_, err := r.ReconstructInvoice(
		t.Context(), envBytes, pathID, paymentHash,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "lookup offer")
}

// TestReconstruct_Memo verifies that the reconstructed invoice carries the
// offer_description as its memo, cut to invoices.MaxMemoSize at a UTF-8 rune
// boundary, and that an offer that expired after the invoice was issued still
// gives the memo.
func TestReconstruct_Memo(t *testing.T) {
	t.Parallel()

	// The two-byte rune starts at the last byte that fits, so a cut at
	// the size limit would split it.
	short := strings.Repeat("a", invoices.MaxMemoSize-1)
	long := short + "é" + "tail"

	createdAt := uint64(time.Now().Unix()) - 100

	tests := []struct {
		name        string
		description string
		expiry      uint64
		wantMemo    string
	}{
		{
			name:        "description",
			description: "coffee",
			wantMemo:    "coffee",
		},
		{
			name:        "description above the size limit",
			description: long,
			wantMemo:    short,
		},
		{
			name:        "offer expired after the invoice",
			description: "coffee",
			expiry:      createdAt + 1,
			wantMemo:    "coffee",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, nodeKey, offer := newTestReconstructor(t)

			decoded := testOffer(t, nodeKey, 1000)
			decoded.OfferDescription = tlv.SomeRecordT(
				tlv.RecordT[tlv.TlvType10, tlv.Blob]{
					Val: []byte(tc.description),
				},
			)
			if tc.expiry != 0 {
				decoded.OfferAbsoluteExpiry = tlv.SomeRecordT(
					tlv.RecordT[tlv.TlvType14,
						bolt12.TUint64]{

						Val: bolt12.TUint64(tc.expiry),
					},
				)
			}

			encoded, err := bolt12.EncodeOfferString(decoded)
			require.NoError(t, err)
			offer.Encoded = encoded

			envBytes, paymentHash := buildTestEnvelope(
				t, nodeKey, offer.Hash, [32]byte{9}, createdAt,
				1000,
			)

			inv, err := r.ReconstructInvoice(
				t.Context(), envBytes, chainhash.Hash{0x01},
				paymentHash,
			)
			require.NoError(t, err)
			require.Equal(t, tc.wantMemo, string(inv.Memo))
			require.NoError(
				t, invoices.ValidateInvoice(inv, paymentHash),
			)
		})
	}
}
