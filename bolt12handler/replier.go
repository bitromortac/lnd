package bolt12handler

import (
	"bytes"
	"context"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/record"
	"github.com/lightningnetwork/lnd/routing/route"
)

// OnionMessageSender is the low-level interface for sending an onion message to
// a peer. This maps to server.SendOnionMessage.
type OnionMessageSender interface {
	SendOnionMessage(ctx context.Context, peerPub [33]byte,
		pathKey *btcec.PublicKey, onion []byte) error
}

// RouteToIntroNode finds a route from this node to the reply path's
// introduction node. Returns nil if the intro node is a direct peer.
type RouteToIntroNode func(introNode route.Vertex) (
	[]route.Vertex, error)

// ServerOnionReplier implements OnionReplier by constructing an onion message
// from the reply path and sending it via the daemon's SendOnionMessage method.
// When the reply path's intro node is not a direct peer, it uses BFS
// pathfinding to route the reply through intermediate nodes.
type ServerOnionReplier struct {
	sender    OnionMessageSender
	findRoute RouteToIntroNode
}

// NewServerOnionReplier creates a new OnionReplier backed by the daemon's onion
// message sender. findRoute may be nil for direct-peer-only operation.
func NewServerOnionReplier(sender OnionMessageSender,
	findRoute RouteToIntroNode) *ServerOnionReplier {

	return &ServerOnionReplier{
		sender:    sender,
		findRoute: findRoute,
	}
}

// HasFindRoute returns true if a route finder has been configured.
func (r *ServerOnionReplier) HasFindRoute() bool {
	return r.findRoute != nil
}

// SetFindRoute sets the route finder for multi-hop reply delivery. Called
// after the router is initialized.
func (r *ServerOnionReplier) SetFindRoute(f RouteToIntroNode) {
	r.findRoute = f
}

// SendInvoiceReply sends the encoded invoice bytes as a type-66 TLV payload via
// the reply path. If the reply path's intro node is not a direct peer, it
// routes through intermediate nodes using BFS pathfinding.
//
// NOTE: This is part of the OnionReplier interface.
func (r *ServerOnionReplier) SendInvoiceReply(ctx context.Context,
	invoiceBytes []byte, replyPath *sphinx.BlindedPath) error {

	if replyPath == nil {
		return fmt.Errorf("no reply path provided")
	}

	if len(replyPath.BlindedHops) == 0 {
		return fmt.Errorf("reply path has no hops")
	}

	// Build the final hop TLV with the invoice payload (type 66).
	finalHopTLVs := []*lnwire.FinalHopTLV{
		{
			TLVType: lnwire.InvoiceNamespaceType,
			Value:   invoiceBytes,
		},
	}

	// Try to find a route to the reply path's introduction node. If we
	// can't (direct peer case), send directly.
	introVertex := route.NewVertex(replyPath.IntroductionPoint)

	var routeHops []route.Vertex
	if r.findRoute != nil {
		var err error
		routeHops, err = r.findRoute(introVertex)
		if err != nil {
			log.Debugf("No route to reply path intro node "+
				"%x, attempting direct send: %v",
				introVertex[:6], err)
		}
	}

	if len(routeHops) > 0 {
		// Multi-hop: build a cleartext path to the intro node,
		// then stitch with the blinded reply path.
		return r.sendViaRoute(
			ctx, routeHops, replyPath, finalHopTLVs,
		)
	}

	// Direct: send the blinded reply path as-is to the intro node.
	return r.sendDirect(ctx, replyPath, finalHopTLVs)
}

// sendDirect sends the reply onion directly to the reply path's intro node.
func (r *ServerOnionReplier) sendDirect(ctx context.Context,
	replyPath *sphinx.BlindedPath,
	finalHopTLVs []*lnwire.FinalHopTLV) error {

	sphinxPath, err := route.OnionMessageBlindedPathToSphinxPath(
		replyPath, nil, finalHopTLVs,
	)
	if err != nil {
		return fmt.Errorf("build sphinx path: %w", err)
	}

	sessionKey, err := btcec.NewPrivateKey()
	if err != nil {
		return fmt.Errorf("generate session key: %w", err)
	}

	onionPkt, err := sphinx.NewOnionPacket(
		sphinxPath, sessionKey, nil,
		sphinx.DeterministicPacketFiller,
		sphinx.WithMaxPayloadSize(
			maxOnionMessagePayloadSize,
		),
	)
	if err != nil {
		return fmt.Errorf("build onion packet: %w", err)
	}

	var buf bytes.Buffer
	if err := onionPkt.Encode(&buf); err != nil {
		return fmt.Errorf("encode onion packet: %w", err)
	}

	var peerPub [33]byte
	copy(
		peerPub[:],
		replyPath.IntroductionPoint.SerializeCompressed(),
	)

	return r.sender.SendOnionMessage(
		ctx, peerPub, replyPath.BlindingPoint, buf.Bytes(),
	)
}

// sendViaRoute sends the reply through intermediate cleartext hops to the
// reply path's introduction node, then the blinded reply path hops.
func (r *ServerOnionReplier) sendViaRoute(ctx context.Context,
	routeHops []route.Vertex, replyPath *sphinx.BlindedPath,
	finalHopTLVs []*lnwire.FinalHopTLV) error {

	log.Infof("Routing invoice reply through %d cleartext "+
		"hop(s) to intro node", len(routeHops))

	// Build a cleartext blinded path from self to the intro node. The
	// last cleartext hop includes a NextBlindingOverride so the intro
	// node switches to the reply path's blinding point.
	sessionKey, err := btcec.NewPrivateKey()
	if err != nil {
		return fmt.Errorf("generate session key: %w", err)
	}

	// The route includes the intro node as the last hop. We build
	// cleartext hops for all nodes EXCEPT the intro node — the intro
	// node is already the first hop of the reply path. The last
	// cleartext hop's next_node_id points to the intro node with a
	// NextBlindingOverride so the intro node switches to the reply
	// path's blinding context.
	//
	// Route: [hop0, hop1, ..., introNode]
	// Cleartext hops: [hop0, hop1, ...] (last one → introNode with
	// override)
	// Then: reply path blinded hops starting at intro node.
	numCleartext := len(routeHops) - 1
	if numCleartext <= 0 {
		// The intro node is our direct peer — no cleartext hops
		// needed, just send directly.
		return r.sendDirect(ctx, replyPath, finalHopTLVs)
	}

	introPub := replyPath.IntroductionPoint

	cleartextHops := make([]*sphinx.HopInfo, numCleartext)
	for i := 0; i < numCleartext; i++ {
		hopPub, err := btcec.ParsePubKey(routeHops[i][:])
		if err != nil {
			return fmt.Errorf("parse hop %d pubkey: %w",
				i, err)
		}

		isLastCleartext := i == numCleartext-1

		var data *record.BlindedRouteData
		if isLastCleartext {
			// Last cleartext hop forwards to the intro node
			// with a blinding override.
			data = record.NewNonFinalBlindedRouteDataOnionMessage(
				fn.NewLeft[
					*btcec.PublicKey,
					lnwire.ShortChannelID,
				](introPub),
				replyPath.BlindingPoint, nil,
			)
		} else {
			nextPub, err := btcec.ParsePubKey(
				routeHops[i+1][:],
			)
			if err != nil {
				return fmt.Errorf(
					"parse hop %d pubkey: %w",
					i+1, err,
				)
			}

			data = record.NewNonFinalBlindedRouteDataOnionMessage(
				fn.NewLeft[
					*btcec.PublicKey,
					lnwire.ShortChannelID,
				](nextPub),
				nil, nil,
			)
		}

		plainText, err := record.EncodeBlindedRouteData(data)
		if err != nil {
			return fmt.Errorf("encode hop %d data: %w",
				i, err)
		}

		cleartextHops[i] = &sphinx.HopInfo{
			NodePub:   hopPub,
			PlainText: plainText,
		}
	}

	cleartextPath, err := sphinx.BuildBlindedPath(
		sessionKey, cleartextHops,
	)
	if err != nil {
		return fmt.Errorf("build cleartext path: %w", err)
	}

	// Concatenate: cleartext hops + reply path blinded hops.
	combinedPath := &sphinx.BlindedPath{
		IntroductionPoint: cleartextPath.Path.IntroductionPoint,
		BlindingPoint:     cleartextPath.Path.BlindingPoint,
		BlindedHops: append(
			cleartextPath.Path.BlindedHops,
			replyPath.BlindedHops...,
		),
	}

	sphinxPath, err := route.OnionMessageBlindedPathToSphinxPath(
		combinedPath, nil, finalHopTLVs,
	)
	if err != nil {
		return fmt.Errorf("build sphinx path: %w", err)
	}

	onionSessionKey, err := btcec.NewPrivateKey()
	if err != nil {
		return fmt.Errorf("generate onion session key: %w", err)
	}

	onionPkt, err := sphinx.NewOnionPacket(
		sphinxPath, onionSessionKey, nil,
		sphinx.DeterministicPacketFiller,
		sphinx.WithMaxPayloadSize(
			maxOnionMessagePayloadSize,
		),
	)
	if err != nil {
		return fmt.Errorf("build onion packet: %w", err)
	}

	var buf bytes.Buffer
	if err := onionPkt.Encode(&buf); err != nil {
		return fmt.Errorf("encode onion packet: %w", err)
	}

	// Send to the first cleartext hop (our direct peer).
	var peerPub [33]byte
	copy(
		peerPub[:],
		combinedPath.IntroductionPoint.SerializeCompressed(),
	)

	return r.sender.SendOnionMessage(
		ctx, peerPub, combinedPath.BlindingPoint, buf.Bytes(),
	)
}
