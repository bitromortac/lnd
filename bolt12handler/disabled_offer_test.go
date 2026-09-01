package bolt12handler

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/stretchr/testify/require"
)

// TestReconstruct_DisabledOffer verifies that the first HTLC for an invoice of
// a disabled offer settles nothing.
func TestReconstruct_DisabledOffer(t *testing.T) {
	t.Parallel()

	r, nodeKey, offer := newTestReconstructor(t)
	offer.IsDisabled = true

	envBytes, paymentHash := buildTestEnvelope(
		t, nodeKey, offer.Hash, [32]byte{0x11},
		uint64(time.Now().Unix()), 1000,
	)

	_, err := r.ReconstructInvoice(
		t.Context(), envBytes, chainhash.Hash{0x01}, paymentHash,
	)
	require.ErrorIs(t, err, ErrOfferDisabled)
}
