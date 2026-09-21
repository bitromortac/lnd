package bolt12handler

import (
	"bytes"
	"context"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

const (
	// maxOnionMessagePayloadSize is the maximum payload size for onion
	// messages. Unlike payment onions (1300 bytes), onion messages can
	// carry up to 32KB per BOLT 7.
	maxOnionMessagePayloadSize = 32768
)

// OnionMessageSender is the low-level interface for sending an onion message to
// a peer. This maps to server.SendOnionMessage.
type OnionMessageSender interface {
	SendOnionMessage(ctx context.Context, peerPub [33]byte,
		pathKey *btcec.PublicKey, onion []byte) error
}

// ServerOnionReplier implements OnionReplier by constructing an onion message
// from the reply path and sending it via the daemon's SendOnionMessage method.
// The reply path's introduction node must be a direct peer.
type ServerOnionReplier struct {
	sender OnionMessageSender
}

// NewServerOnionReplier creates a new OnionReplier backed by the daemon's onion
// message sender.
func NewServerOnionReplier(
	sender OnionMessageSender) *ServerOnionReplier {

	return &ServerOnionReplier{
		sender: sender,
	}
}

// SendInvoiceReply sends the encoded invoice bytes as a type-66 TLV payload via
// the reply path.
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

	// Send the blinded reply path as-is to the intro node.
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
