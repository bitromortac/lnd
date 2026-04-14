package blindedpath

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/stretchr/testify/require"
)

// testKey generates a deterministic private key from a seed byte.
func testKey(t *testing.T, seed byte) *btcec.PrivateKey {
	t.Helper()

	var keyBytes [32]byte
	keyBytes[0] = seed
	keyBytes[31] = 1

	key, _ := btcec.PrivKeyFromBytes(keyBytes[:])

	return key
}

// testVertex returns a route.Vertex from a private key.
func testVertex(key *btcec.PrivateKey) route.Vertex {
	var v route.Vertex
	copy(v[:], key.PubKey().SerializeCompressed())

	return v
}

// TestBuildBlindedMessagePaths_SingleHop verifies that a self-only path (where
// the destination is the introduction node) produces a single blinded hop with
// empty or path_id-bearing encrypted data.
func TestBuildBlindedMessagePaths_SingleHop(t *testing.T) {
	t.Parallel()

	selfKey := testKey(t, 1)
	selfVertex := testVertex(selfKey)

	cfg := &BuildBlindedMessagePathCfg{
		FindRoutes: func() ([]*route.Route, error) {
			return []*route.Route{
				{SourcePubKey: selfVertex},
			}, nil
		},
	}

	paths, err := BuildBlindedMessagePaths(cfg)
	require.NoError(t, err)
	require.Len(t, paths, 1)

	path := paths[0]
	require.NotNil(t, path.Path)
	require.Len(t, path.Path.BlindedHops, 1)
	require.Equal(
		t, selfKey.PubKey(), path.Path.IntroductionPoint,
	)
}

// TestBuildBlindedMessagePaths_MultiHop verifies that a multi-hop route
// produces the correct number of blinded hops.
func TestBuildBlindedMessagePaths_MultiHop(t *testing.T) {
	t.Parallel()

	introKey := testKey(t, 1)
	hop1Key := testKey(t, 2)
	destKey := testKey(t, 3)

	introVertex := testVertex(introKey)
	hop1Vertex := testVertex(hop1Key)
	destVertex := testVertex(destKey)

	cfg := &BuildBlindedMessagePathCfg{
		FindRoutes: func() ([]*route.Route, error) {
			return []*route.Route{
				{
					SourcePubKey: introVertex,
					Hops: []*route.Hop{
						{PubKeyBytes: hop1Vertex},
						{PubKeyBytes: destVertex},
					},
				},
			}, nil
		},
	}

	paths, err := BuildBlindedMessagePaths(cfg)
	require.NoError(t, err)
	require.Len(t, paths, 1)

	path := paths[0]

	// 3 nodes: intro, hop1, dest → 3 blinded hops.
	require.Len(t, path.Path.BlindedHops, 3)
	require.Equal(
		t, introKey.PubKey(), path.Path.IntroductionPoint,
	)
}

// TestBuildBlindedMessagePaths_WithPathID verifies that providing a PathID
// still produces a valid blinded path. The path_id is embedded in the final
// hop's encrypted data; we verify the path is structurally valid. Decryption
// of the embedded path_id is tested at the integration level where the
// sphinx.Router is available to decrypt the hop data.
func TestBuildBlindedMessagePaths_WithPathID(t *testing.T) {
	t.Parallel()

	selfKey := testKey(t, 1)
	selfVertex := testVertex(selfKey)

	pathID := []byte("test-path-id-for-offer-matching")

	cfg := &BuildBlindedMessagePathCfg{
		FindRoutes: func() ([]*route.Route, error) {
			return []*route.Route{
				{SourcePubKey: selfVertex},
			}, nil
		},
		PathID: pathID,
	}

	paths, err := BuildBlindedMessagePaths(cfg)
	require.NoError(t, err)
	require.Len(t, paths, 1)

	path := paths[0]
	require.Len(t, path.Path.BlindedHops, 1)

	// The final hop's ciphertext should be non-empty (contains the
	// encrypted path_id).
	lastHop := path.Path.BlindedHops[0]
	require.NotEmpty(t, lastHop.CipherText)
}

// TestBuildBlindedMessagePaths_MultiplePaths verifies that multiple candidate
// routes each produce a separate blinded path.
func TestBuildBlindedMessagePaths_MultiplePaths(t *testing.T) {
	t.Parallel()

	key1 := testKey(t, 1)
	key2 := testKey(t, 2)
	destKey := testKey(t, 3)

	vertex1 := testVertex(key1)
	vertex2 := testVertex(key2)
	destVertex := testVertex(destKey)

	cfg := &BuildBlindedMessagePathCfg{
		FindRoutes: func() ([]*route.Route, error) {
			return []*route.Route{
				{
					SourcePubKey: vertex1,
					Hops: []*route.Hop{
						{PubKeyBytes: destVertex},
					},
				},
				{
					SourcePubKey: vertex2,
					Hops: []*route.Hop{
						{PubKeyBytes: destVertex},
					},
				},
			}, nil
		},
	}

	paths, err := BuildBlindedMessagePaths(cfg)
	require.NoError(t, err)
	require.Len(t, paths, 2)

	// Each path should have a different introduction point.
	require.NotEqual(
		t,
		paths[0].Path.IntroductionPoint,
		paths[1].Path.IntroductionPoint,
	)
}

// TestBuildBlindedMessagePaths_EmptyRoutes verifies that an error is returned
// when no routes are available.
func TestBuildBlindedMessagePaths_EmptyRoutes(t *testing.T) {
	t.Parallel()

	cfg := &BuildBlindedMessagePathCfg{
		FindRoutes: func() ([]*route.Route, error) {
			return nil, nil
		},
	}

	_, err := BuildBlindedMessagePaths(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "could not find any routes")
}
