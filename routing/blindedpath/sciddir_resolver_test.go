package blindedpath

import (
	"errors"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/stretchr/testify/require"
)

// stubFetcher returns a FetchChannelEdgeByID that returns a fixed edge for
// matchID and ErrEdgeNotFound for everything else.
func stubFetcher(t *testing.T, matchID uint64,
	edge *models.ChannelEdgeInfo) FetchChannelEdgeByID {

	t.Helper()

	return func(chanID uint64) (*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {

		if chanID != matchID {
			return nil, nil, nil, graphdb.ErrEdgeNotFound
		}

		return edge, nil, nil, nil
	}
}

// keyVertex generates a fresh secp256k1 key and returns the route.Vertex view
// of its compressed serialisation.
func keyVertex(t *testing.T) (*btcec.PublicKey, route.Vertex) {
	t.Helper()

	priv, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	var v route.Vertex
	copy(v[:], priv.PubKey().SerializeCompressed())

	return priv.PubKey(), v
}

// TestSciddirResolverDispatch pins channel_announcement-style ordering: a
// resolver that finds the channel returns NodeKey1 for direction 0 and
// NodeKey2 for direction 1, and surfaces ErrUnknownSciddir as a typed
// sentinel when the channel is not in the local graph.
func TestSciddirResolverDispatch(t *testing.T) {
	t.Parallel()

	pub1, vertex1 := keyVertex(t)
	pub2, vertex2 := keyVertex(t)

	const chanID uint64 = 0x0102030405060708

	edge := &models.ChannelEdgeInfo{
		ChannelID:     chanID,
		NodeKey1Bytes: vertex1,
		NodeKey2Bytes: vertex2,
	}

	scid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

	tests := []struct {
		name      string
		direction byte
		fetcher   FetchChannelEdgeByID
		wantPub   *btcec.PublicKey
		wantErr   error
		wantMsg   string
	}{
		{
			name:      "direction 0 returns lesser pubkey",
			direction: 0,
			fetcher:   stubFetcher(t, chanID, edge),
			wantPub:   pub1,
		},
		{
			name:      "direction 1 returns greater pubkey",
			direction: 1,
			fetcher:   stubFetcher(t, chanID, edge),
			wantPub:   pub2,
		},
		{
			name:      "unknown channel surfaces sentinel",
			direction: 0,
			fetcher:   stubFetcher(t, 0xdead, edge),
			wantErr:   lnwire.ErrUnknownSciddir,
		},
		{
			name:      "invalid direction rejected",
			direction: 0x02,
			fetcher:   stubFetcher(t, chanID, edge),
			wantMsg:   "invalid sciddir direction",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := NewSciddirResolver(tc.fetcher)
			got, err := r.ResolveSciddir(tc.direction, scid)

			if tc.wantErr != nil {
				require.Error(t, err)
				require.True(t,
					errors.Is(err, tc.wantErr),
					"expected sentinel %v, got %v",
					tc.wantErr, err)

				return
			}
			if tc.wantMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantMsg)

				return
			}

			require.NoError(t, err)
			require.True(t, tc.wantPub.IsEqual(got))
		})
	}
}
