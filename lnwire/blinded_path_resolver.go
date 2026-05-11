package lnwire

import (
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	sphinx "github.com/lightningnetwork/lightning-onion"
)

// ErrUnknownSciddir is returned by an IntroNodeResolver when the referenced
// short channel ID is not present in the local channel graph (stale gossip,
// private channel, closed channel). RPC handlers map this to a precondition
// failure; internal callers compare via errors.Is so the resolver
// implementation can wrap freely without breaking the contract.
var ErrUnknownSciddir = errors.New("sciddir introduction node not found in " +
	"channel graph")

// IntroNodeResolver resolves a sciddir introduction node to the endpoint
// pubkey identified by the (direction, scid) pair, using channel_announcement
// ordering (0 = lesser pubkey, 1 = greater). The implementation lives in
// routing/blindedpath alongside the channel-graph lookup; this package only
// declares the interface so callers of (*BlindedPath).ToSphinx do not need a
// routing import.
type IntroNodeResolver interface {
	ResolveSciddir(direction byte, scid [scidLen]byte) (*btcec.PublicKey,
		error)
}

// NewBlindedPathFromSphinx rebuilds a sphinx-form blinded path as the lnwire
// equivalent. Callers that build paths via sphinx.BuildBlindedPath translate
// at the boundary using this helper before storing the result on
// OnionMessagePayload.ReplyPath or any TLV field.
func NewBlindedPathFromSphinx(p *sphinx.BlindedPath) (*BlindedPath, error) {
	if p == nil {
		return nil, nil
	}

	hops := make([]BlindedHop, len(p.BlindedHops))
	for i, h := range p.BlindedHops {
		hops[i].BlindedNodeID = h.BlindedNodePub
		hops[i].EncryptedData = h.CipherText
	}

	introNode, err := NewPubkeyIntro(p.IntroductionPoint)
	if err != nil {
		return nil, err
	}

	return &BlindedPath{
		IntroductionNode: introNode,
		BlindingPoint:    p.BlindingPoint,
		Hops:             hops,
	}, nil
}

// ToSphinx assembles the sphinx-form representation used by the routing layer.
// Pubkey-form introduction nodes are passed through directly; sciddir-form
// introduction nodes are dispatched through the resolver.
func (p *BlindedPath) ToSphinx(r IntroNodeResolver) (*sphinx.BlindedPath,
	error) {

	if p.IntroductionNode == nil {
		return nil, fmt.Errorf("nil intro node")
	}

	introPub, err := ResolveIntroductionNode(p.IntroductionNode, r)
	if err != nil {
		return nil, err
	}

	if p.BlindingPoint == nil {
		return nil, fmt.Errorf("nil blinding point")
	}

	hops := make([]*sphinx.BlindedHopInfo, len(p.Hops))
	for i := range p.Hops {
		if p.Hops[i].BlindedNodeID == nil {
			return nil, fmt.Errorf("nil blinded hop %d", i)
		}

		hops[i] = &sphinx.BlindedHopInfo{
			BlindedNodePub: p.Hops[i].BlindedNodeID,
			CipherText:     p.Hops[i].EncryptedData,
		}
	}

	return &sphinx.BlindedPath{
		IntroductionPoint: introPub,
		BlindingPoint:     p.BlindingPoint,
		BlindedHops:       hops,
	}, nil
}

// ResolveIntroductionNode returns the pubkey of an IntroductionNode. A
// nil resolver is only valid for PubkeyIntro inputs.
func ResolveIntroductionNode(intro IntroductionNode,
	r IntroNodeResolver) (*btcec.PublicKey, error) {

	switch v := intro.(type) {
	case PubkeyIntro:
		if v.Pubkey == nil {
			return nil, fmt.Errorf("%w: nil pubkey",
				ErrInvalidIntroNode)
		}

		return v.Pubkey, nil

	case SciddirIntro:
		if r == nil {
			return nil, fmt.Errorf("sciddir intro requires a " +
				"resolver")
		}

		pub, err := r.ResolveSciddir(v.Direction, v.SCID)
		if err != nil {
			return nil, err
		}

		return pub, nil

	default:
		return nil, fmt.Errorf("%w: %T", ErrInvalidIntroNode, intro)
	}
}
