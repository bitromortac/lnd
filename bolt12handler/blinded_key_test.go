package bolt12handler

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/stretchr/testify/require"
)

// TestBlindedKeyMatchesPathBlindedNodeID builds a real route-blinding path
// and checks that the key derived from the arrival path key is the
// blinded_node_id the path publishes for that hop, and signs for it. This is
// the property the payer relies on when an offer carries offer_paths instead
// of offer_issuer_id.
func TestBlindedKeyMatchesPathBlindedNodeID(t *testing.T) {
	t.Parallel()

	sessionKey := testKey(t)
	nodeKey := testKey2(t)

	path, err := sphinx.BuildBlindedPath(sessionKey, []*sphinx.HopInfo{{
		NodePub:   nodeKey.PubKey(),
		PlainText: []byte{0},
	}})
	require.NoError(t, err)
	require.Len(t, path.Path.BlindedHops, 1)

	// A single-hop path is entered with the session key itself, so that is
	// the ephemeral key this node sees on the incoming message.
	published := path.Path.BlindedHops[0].BlindedNodePub
	for _, signer := range []Signer{
		NewPrivKeySigner(nodeKey), testKeyRingSigner(nodeKey),
	} {
		derived, err := blindedNodeID(signer, sessionKey.PubKey())
		require.NoError(t, err)
		require.True(t, derived.IsEqual(published),
			"derived blinded key does not match the path's "+
				"blinded_node_id")
	}
}

// TestBlindedKeyRejectsMissingPathKey pins that the derivation refuses to
// invent a key when the path key is absent, so a caller cannot silently sign
// with the wrong identity.
func TestBlindedKeyRejectsMissingPathKey(t *testing.T) {
	t.Parallel()

	_, err := blindedKeyRef(NewPrivKeySigner(testKey(t)), nil)
	require.Error(t, err)
}

// TestBlindedKeyIsPathKeySpecific verifies that a different ephemeral key
// yields a different blinded identity, which is what stops one blinded path's
// key from answering for another.
func TestBlindedKeyIsPathKeySpecific(t *testing.T) {
	t.Parallel()

	signer := NewPrivKeySigner(testKey(t))

	first, err := blindedNodeID(signer, testSeededKey(t, 7).PubKey())
	require.NoError(t, err)

	second, err := blindedNodeID(signer, testSeededKey(t, 8).PubKey())
	require.NoError(t, err)

	require.False(t, first.IsEqual(second))
}

// testSeededKey builds a deterministic private key from a single seed byte.
func testSeededKey(t *testing.T, seedByte int) *btcec.PrivateKey {
	t.Helper()

	var seed [32]byte
	for i := range seed {
		seed[i] = byte(i + seedByte)
	}

	key, _ := btcec.PrivKeyFromBytes(seed[:])

	return key
}
