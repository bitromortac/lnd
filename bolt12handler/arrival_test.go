package bolt12handler

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// newPathKey returns a fresh key that stands for the path key a message
// arrives under.
func newPathKey(t *testing.T) *btcec.PublicKey {
	t.Helper()

	key, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	return key.PubKey()
}

// setOfferPaths gives the request offer paths whose final hops name the given
// blinded node ids.
func setOfferPaths(ir *bolt12.InvoiceRequest, finals ...*btcec.PublicKey) {
	paths := lnwire.BlindedPaths{}
	for _, final := range finals {
		paths.Paths = append(paths.Paths, lnwire.BlindedPath{
			Hops: []lnwire.BlindedHop{{BlindedNodeID: final}},
		})
	}

	ir.OfferPaths = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType16, lnwire.BlindedPaths]{Val: paths},
	)
}

// TestCheckArrivalPath verifies that a request for an offer with offer_paths
// is answered only when it arrived on one of them.
func TestCheckArrivalPath(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	signer := newPrivKeySigner(nodeKey)
	offer := testOffer(t, nodeKey, 1000)

	payerKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	// The node reached on the offer's second path.
	arrival := newPathKey(t)
	arrivalID, err := signer.BlindedNodePubKey(arrival)
	require.NoError(t, err)

	otherID, err := signer.BlindedNodePubKey(newPathKey(t))
	require.NoError(t, err)

	// An offer without offer_paths has no path to check.
	ir := testInvoiceRequest(t, offer, payerKey, 0)
	require.NoError(t, CheckArrivalPath(ir, signer, arrival))

	// The request came along one of the offer paths.
	setOfferPaths(ir, otherID, arrivalID)
	require.NoError(t, CheckArrivalPath(ir, signer, arrival))

	// The request came along a path of another offer.
	setOfferPaths(ir, otherID)
	require.ErrorIs(
		t, CheckArrivalPath(ir, signer, arrival), ErrWrongArrivalPath,
	)

	// No arrival path key at all.
	require.ErrorIs(
		t, CheckArrivalPath(ir, signer, nil), ErrWrongArrivalPath,
	)
}
