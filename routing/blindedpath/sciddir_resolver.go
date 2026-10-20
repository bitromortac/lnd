package blindedpath

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
)

// FetchChannelEdgeByID is the channel-graph lookup used by SciddirResolver.
// It mirrors the signature already used by BuildBlindedPathCfg so callers can
// share a single graph-backed callback across both helpers.
type FetchChannelEdgeByID func(chanID uint64) (*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error)

// SciddirResolver resolves sciddir blinded-path introduction nodes to the
// endpoint pubkey identified by (direction, scid) using channel_announcement
// ordering: direction 0 selects the numerically lesser pubkey
// (NodeKey1Bytes), direction 1 selects the greater (NodeKey2Bytes). It
// implements lnwire.IntroNodeResolver so a single graph-backed instance
// services every BOLT 4 / BOLT 12 conversion site.
type SciddirResolver struct {
	fetchChannelEdge FetchChannelEdgeByID
}

// NewSciddirResolver returns a resolver that consults the given
// channel-graph lookup. Constructor injection keeps lnwire's IntroNodeResolver
// interface free of routing-package types and lets tests substitute a stub
// without standing up a full graph.
func NewSciddirResolver(
	fetchChannelEdge FetchChannelEdgeByID) *SciddirResolver {

	return &SciddirResolver{fetchChannelEdge: fetchChannelEdge}
}

// ResolveSciddir looks up the channel referenced by scid and returns the
// endpoint pubkey selected by direction. ErrUnknownSciddir is returned when
// the channel is not present in the local graph (stale gossip, private
// channel, or closed channel).
func (r *SciddirResolver) ResolveSciddir(direction byte,
	scid [8]byte) (*btcec.PublicKey, error) {

	chanID := binary.BigEndian.Uint64(scid[:])

	info, _, _, err := r.fetchChannelEdge(chanID)
	if errors.Is(err, graphdb.ErrEdgeNotFound) {
		return nil, fmt.Errorf("%w: scid=%d", lnwire.ErrUnknownSciddir,
			chanID)
	}
	if err != nil {
		return nil, fmt.Errorf("fetch channel %d: %w", chanID, err)
	}

	var pubBytes [33]byte
	switch direction {
	case 0:
		pubBytes = info.NodeKey1Bytes
	case 1:
		pubBytes = info.NodeKey2Bytes
	default:
		return nil, fmt.Errorf("invalid sciddir direction 0x%02x",
			direction)
	}

	pub, err := btcec.ParsePubKey(pubBytes[:])
	if err != nil {
		return nil, fmt.Errorf("parse channel %d node key: %w", chanID,
			err)
	}

	return pub, nil
}
