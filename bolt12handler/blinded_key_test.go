package bolt12handler

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/stretchr/testify/require"
)

// TestBlindPrivKeyMatchesPathBlindedNodeID builds a real route-blinding path
// and checks that the key derived from the arrival path key signs for the
// blinded_node_id the path publishes for that hop. This is the property the
// payer relies on when an offer carries offer_paths instead of
// offer_issuer_id.
func TestBlindPrivKeyMatchesPathBlindedNodeID(t *testing.T) {
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
	blindedPriv, err := blindPrivKey(nodeKey, sessionKey.PubKey())
	require.NoError(t, err)

	published := path.Path.BlindedHops[0].BlindedNodePub
	require.True(t, blindedPriv.PubKey().IsEqual(published),
		"derived blinded key does not match the path's "+
			"blinded_node_id")
}

// TestBlindPrivKeyRejectsMissingKeys pins that the derivation refuses to
// invent a key when either input is absent, so a caller cannot silently sign
// with the wrong identity.
func TestBlindPrivKeyRejectsMissingKeys(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)

	_, err := blindPrivKey(nodeKey, nil)
	require.Error(t, err)

	_, err = blindPrivKey(nil, nodeKey.PubKey())
	require.Error(t, err)
}

// TestBlindPrivKeyIsPathKeySpecific verifies that a different ephemeral key
// yields a different blinded identity, which is what stops one blinded path's
// key from answering for another.
func TestBlindPrivKeyIsPathKeySpecific(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)

	first, err := blindPrivKey(nodeKey, testSeededKey(t, 7).PubKey())
	require.NoError(t, err)

	second, err := blindPrivKey(nodeKey, testSeededKey(t, 8).PubKey())
	require.NoError(t, err)

	require.False(t, first.PubKey().IsEqual(second.PubKey()))
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
