package bolt12handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/onionmessage"
	"github.com/lightningnetwork/lnd/record"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/subscribe"
	"github.com/lightningnetwork/lnd/tlv"
)

const (
	// maxOnionMessagePayloadSize is the maximum payload size for onion
	// messages. Unlike payment onions (1300 bytes), onion messages can
	// carry up to 32KB per BOLT 7.
	maxOnionMessagePayloadSize = 32768
)

// ReplyPathBuilder constructs a blinded reply path for the sender to include
// in invoice requests. The receiver will use this path to send the invoice
// back. Implementations range from a single-hop path (direct peer) to
// multi-hop blinded paths (routed messages).
type ReplyPathBuilder interface {
	// BuildReplyPath returns a blinded path that points back to the
	// sender. The receiver will use this as the reply_path for the
	// invoice onion message.
	BuildReplyPath() (*sphinx.BlindedPathInfo, error)
}

// SingleHopReplyPathBuilder builds a trivial single-hop reply path where the
// sender is both introduction node and destination. Suitable for the
// direct-peer case.
type SingleHopReplyPathBuilder struct {
	NodePubKey *btcec.PublicKey
}

// BuildReplyPath creates a single-hop blinded path back to the sender.
func (b *SingleHopReplyPathBuilder) BuildReplyPath() (
	*sphinx.BlindedPathInfo, error) {

	return BuildSingleHopReplyPath(b.NodePubKey)
}

// MultiHopReplyPathBuilder attempts to build a multi-hop blinded reply path
// using the provided BuildPaths function. If no multi-hop paths are found, it
// falls back to a single-hop path.
type MultiHopReplyPathBuilder struct {
	// NodePubKey is the node's identity public key, used as fallback for
	// single-hop path construction.
	NodePubKey *btcec.PublicKey

	// BuildPaths attempts to find and construct multi-hop blinded message
	// paths to this node.
	BuildPaths func() ([]*sphinx.BlindedPathInfo, error)
}

// BuildReplyPath attempts to build a multi-hop blinded reply path. If no
// multi-hop paths are available, it falls back to a single-hop path.
func (b *MultiHopReplyPathBuilder) BuildReplyPath() (
	*sphinx.BlindedPathInfo, error) {

	paths, err := b.BuildPaths()
	if err != nil {
		log.Debugf("Multi-hop reply path construction failed, "+
			"falling back to single-hop: %v", err)

		return BuildSingleHopReplyPath(b.NodePubKey)
	}

	if len(paths) == 0 {
		log.Debugf("No multi-hop reply paths found, falling " +
			"back to single-hop")

		return BuildSingleHopReplyPath(b.NodePubKey)
	}

	numHops := len(paths[0].Path.BlindedHops)
	log.Infof("Using multi-hop reply path with %d blinded hop(s)",
		numHops)

	return paths[0], nil
}

// RequestOption configures optional fields on an invoice request.
type RequestOption func(*requestConfig)

type requestConfig struct {
	amountMsat uint64
	quantity   uint64
	payerNote  string
}

// WithAmount sets invreq_amount on the invoice request. Required when the offer
// has no fixed amount.
func WithAmount(msat uint64) RequestOption {
	return func(c *requestConfig) {
		c.amountMsat = msat
	}
}

// WithQuantity sets invreq_quantity on the invoice request. Required when the
// offer supports quantity selection.
func WithQuantity(qty uint64) RequestOption {
	return func(c *requestConfig) {
		c.quantity = qty
	}
}

// WithPayerNote sets invreq_payer_note on the invoice request.
func WithPayerNote(note string) RequestOption {
	return func(c *requestConfig) {
		c.payerNote = note
	}
}

// BuildInvoiceRequest constructs a signed BOLT 12 invoice request from a
// decoded offer. It generates an ephemeral keypair for proof of payer, mirrors
// all offer fields, and signs the request. Returns the signed request and the
// ephemeral private key.
func BuildInvoiceRequest(offer *bolt12.Offer, opts ...RequestOption) (
	*bolt12.InvoiceRequest, *btcec.PrivateKey, error) {

	cfg := &requestConfig{}
	for _, o := range opts {
		o(cfg)
	}

	// Generate ephemeral keypair for invreq_payer_id.
	payerKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, nil, fmt.Errorf("generate payer key: %w", err)
	}

	metadata := make([]byte, 32)
	if _, err := rand.Read(metadata); err != nil {
		return nil, nil, fmt.Errorf("generate metadata: %w", err)
	}

	// Pay on the offer's chain: use the first listed offer_chains entry, or
	// Bitcoin mainnet when offer_chains is absent. The constructor sets
	// invreq_chain only for non-bitcoin chains (SHOULD omit for mainnet) and
	// the writer validation enforces that the chain is one the offer lists.
	chain := bolt12.BitcoinMainnetGenesisHash()
	offer.OfferChains.WhenSome(
		func(r tlv.RecordT[tlv.TlvType2, bolt12.ChainsRecord]) {
			if len(r.Val.Chains) > 0 {
				chain = r.Val.Chains[0]
			}
		},
	)

	ir, err := bolt12.NewInvoiceRequestFromOffer(
		offer, payerKey.PubKey(), metadata, chain,
	)
	if err != nil {
		return nil, nil, err
	}

	if cfg.amountMsat > 0 {
		amt := bolt12.TUint64(cfg.amountMsat)
		ir.InvreqAmount = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType82, bolt12.TUint64]{
				Val: amt,
			},
		)
	}

	if cfg.quantity > 0 {
		qty := bolt12.TUint64(cfg.quantity)
		ir.InvreqQuantity = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType86, bolt12.TUint64]{
				Val: qty,
			},
		)
	}

	if cfg.payerNote != "" {
		ir.InvreqPayerNote = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType89, tlv.Blob]{
				Val: []byte(cfg.payerNote),
			},
		)
	}

	// Encode emits canonical bytes and repopulates rawTLVs, removing
	// the previous decode-after-encode dance.
	if _, err := ir.Encode(); err != nil {
		return nil, nil, fmt.Errorf("encode invreq: %w", err)
	}

	// Sign with the ephemeral payer key.
	sig, err := bolt12.SignInvoiceRequest(ir, payerKey)
	if err != nil {
		return nil, nil, fmt.Errorf("sign invreq: %w", err)
	}

	ir.Signature = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType240, [64]byte](sig),
	)

	return ir, payerKey, nil
}

// BuildSingleHopReplyPath creates a single-hop blinded path back to the sender
// for the direct-peer case. The sender is both introduction node and
// destination.
func BuildSingleHopReplyPath(nodePubKey *btcec.PublicKey) (
	*sphinx.BlindedPathInfo, error) {

	sessionKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("generate session key: %w", err)
	}

	hops := []*sphinx.HopInfo{
		{
			NodePub:   nodePubKey,
			PlainText: encodeEmptyRouteData(),
		},
	}

	blindedPath, err := sphinx.BuildBlindedPath(sessionKey, hops)
	if err != nil {
		return nil, fmt.Errorf("build blinded path: %w", err)
	}

	return blindedPath, nil
}

// SendInvoiceRequest sends a signed invoice request to the recipient via onion
// message. It wraps the request in a type-64 TLV payload and sends it along
// the provided forward path with the provided reply path. The forward path is
// a blinded path to the recipient (single-hop for direct peers, multi-hop for
// routed messages).
func SendInvoiceRequest(ctx context.Context, invreqBytes []byte,
	forwardPath *sphinx.BlindedPathInfo,
	replyPath *sphinx.BlindedPathInfo,
	sender OnionMessageSender) error {

	finalHopTLVs := []*lnwire.FinalHopTLV{
		{
			TLVType: lnwire.InvoiceRequestNamespaceType,
			Value:   invreqBytes,
		},
	}

	replyBlindedPath, err := lnwire.NewBlindedPathFromSphinx(replyPath.Path)
	if err != nil {
		return fmt.Errorf("build reply path: %w", err)
	}

	sphinxPath, err := route.OnionMessageBlindedPathToSphinxPath(
		forwardPath.Path,
		replyBlindedPath, finalHopTLVs,
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
		sphinx.DeterministicPacketFiller, sphinx.WithMaxPayloadSize(
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
		peerPub[:], forwardPath.Path.IntroductionPoint.
			SerializeCompressed(),
	)

	return sender.SendOnionMessage(
		ctx, peerPub, forwardPath.Path.BlindingPoint, buf.Bytes(),
	)
}

// BuildForwardPath constructs a blinded path from the sender to the recipient
// for forwarding an onion message. For a direct peer, this is a single-hop
// path. For a multi-hop route, the cleartext hops are encoded as blinded hops
// with next_node_id routing data.
func BuildForwardPath(recipientPubKey *btcec.PublicKey,
	route []route.Vertex) (*sphinx.BlindedPathInfo, error) {

	sessionKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("generate session key: %w", err)
	}

	// Build hop infos. For multi-hop, each intermediate hop's encrypted
	// data contains next_node_id. The final hop gets empty route data.
	hops := make([]*sphinx.HopInfo, 0, len(route)+1)

	for i, vertex := range route {
		hopPub, err := btcec.ParsePubKey(vertex[:])
		if err != nil {
			return nil, fmt.Errorf("parse hop %d pubkey: %w",
				i, err)
		}

		isFinal := i == len(route)-1
		var plainText []byte

		if isFinal {
			plainText = encodeEmptyRouteData()
		} else {
			nextPub, err := btcec.ParsePubKey(
				route[i+1][:],
			)
			if err != nil {
				return nil, fmt.Errorf(
					"parse hop %d pubkey: %w",
					i+1, err,
				)
			}

			data := record.NewNonFinalBlindedRouteDataOnionMessage(
				fn.NewLeft[
					*btcec.PublicKey,
					lnwire.ShortChannelID,
				](nextPub),
				nil, nil,
			)

			plainText, err = record.EncodeBlindedRouteData(
				data,
			)
			if err != nil {
				return nil, fmt.Errorf(
					"encode hop %d data: %w", i, err,
				)
			}
		}

		hops = append(hops, &sphinx.HopInfo{
			NodePub:   hopPub,
			PlainText: plainText,
		})
	}

	// If no route hops, build a single-hop direct path.
	if len(hops) == 0 {
		hops = append(hops, &sphinx.HopInfo{
			NodePub:   recipientPubKey,
			PlainText: encodeEmptyRouteData(),
		})
	}

	return sphinx.BuildBlindedPath(sessionKey, hops)
}

// ValidateInvoiceReply validates a received BOLT 12 invoice against the
// original invoice request and offer. It performs structural validation,
// signature verification, byte-for-byte field matching, and invoice_node_id
// verification. activeChain is the genesis hash the sender is willing to
// settle on; it gates the spec invreq_chain reader rule.
func ValidateInvoiceReply(inv *bolt12.Invoice, req *bolt12.InvoiceRequest,
	offer *bolt12.Offer, activeChain [32]byte) error {

	if err := bolt12.ValidateInvoiceRead(
		inv, activeChain, bolt12.InvoiceFeatureCatalogues{
			Invoice: bolt12.Bolt12Features,
			Blinded: bolt12.Bolt12Features,
		},
	); err != nil {
		return fmt.Errorf("validate invoice: %w", err)
	}

	if err := bolt12.VerifyInvoice(inv); err != nil {
		return fmt.Errorf("verify invoice signature: %w", err)
	}

	if err := bolt12.ValidateInvoiceAgainstRequest(
		inv, req,
	); err != nil {

		return fmt.Errorf("invoice/request mismatch: %w", err)
	}

	if err := verifyInvoiceNodeID(inv, offer); err != nil {
		return err
	}

	return nil
}

// verifyInvoiceNodeID checks that the invoice's signing key matches the offer
// issuer's identity.
func verifyInvoiceNodeID(inv *bolt12.Invoice, offer *bolt12.Offer) error {

	var invoiceNodeID []byte
	inv.InvoiceNodeID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType176, *btcec.PublicKey]) {
			if r.Val != nil {
				invoiceNodeID = r.Val.SerializeCompressed()
			}
		},
	)

	// When offer_issuer_id is present, invoice_node_id must match.
	var issuerID []byte
	offer.OfferIssuerID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType22, *btcec.PublicKey]) {
			issuerID = r.Val.SerializeCompressed()
		},
	)

	if issuerID != nil && !bytes.Equal(invoiceNodeID, issuerID) {
		return fmt.Errorf("invoice_node_id does not match " +
			"offer_issuer_id")
	}

	// TODO(bolt12): When offer_paths is present but offer_issuer_id is
	// absent, verify invoice_node_id matches the final blinded_node_id.
	// Deferred to Layer 5.

	return nil
}

// WaitForInvoiceReply subscribes to onion message updates and waits for an
// invoice reply (TLV type 66). Returns the raw invoice bytes.
func WaitForInvoiceReply(ctx context.Context, msgServer *subscribe.Server,
	timeout time.Duration) ([]byte, error) {

	client, err := msgServer.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("subscribe to onion messages: %w", err)
	}
	defer client.Cancel()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case update, ok := <-client.Updates():
			if !ok {
				return nil, fmt.Errorf("onion message " +
					"subscription closed")
			}

			msg, isOnion := update.(*onionmessage.OnionMessageUpdate)
			if !isOnion {
				continue
			}

			// Look for an invoice payload (TLV type 66).
			invoiceBytes, hasInvoice := msg.CustomRecords[uint64(
				lnwire.InvoiceNamespaceType,
			)]
			if !hasInvoice {
				continue
			}

			return invoiceBytes, nil

		case <-timer.C:
			return nil, fmt.Errorf("timeout waiting for invoice " +
				"reply")

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// BuildForwardPathToBlinded constructs a combined path: cleartext hops from
// the sender to the blinded path's introduction node, then the blinded hops.
// The last cleartext hop includes NextBlindingOverride to hand off the
// blinding context to the blinded segment.
func BuildForwardPathToBlinded(routeHops []route.Vertex,
	blindedPath *sphinx.BlindedPath) (*sphinx.BlindedPathInfo, error) {

	// If the route leads directly to the intro node (1 hop = intro
	// node itself), use the blinded path as-is.
	if len(routeHops) <= 1 {
		return &sphinx.BlindedPathInfo{
			Path: blindedPath,
		}, nil
	}

	sessionKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("generate session key: %w", err)
	}

	// Build cleartext hops for all nodes BEFORE the intro node.
	// The intro node is the last element of routeHops.
	numCleartext := len(routeHops) - 1
	cleartextHops := make([]*sphinx.HopInfo, numCleartext)

	for i := 0; i < numCleartext; i++ {
		hopPub, err := btcec.ParsePubKey(routeHops[i][:])
		if err != nil {
			return nil, fmt.Errorf("parse hop %d: %w", i, err)
		}

		isLast := i == numCleartext-1

		var data *record.BlindedRouteData
		if isLast {
			data = record.NewNonFinalBlindedRouteDataOnionMessage(
				fn.NewLeft[
					*btcec.PublicKey,
					lnwire.ShortChannelID,
				](blindedPath.IntroductionPoint),
				blindedPath.BlindingPoint, nil,
			)
		} else {
			nextPub, err := btcec.ParsePubKey(
				routeHops[i+1][:],
			)
			if err != nil {
				return nil, fmt.Errorf(
					"parse hop %d: %w", i+1, err,
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
			return nil, fmt.Errorf("encode hop %d: %w", i, err)
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
		return nil, fmt.Errorf("build cleartext path: %w", err)
	}

	combined := &sphinx.BlindedPath{
		IntroductionPoint: cleartextPath.Path.IntroductionPoint,
		BlindingPoint:     cleartextPath.Path.BlindingPoint,
		BlindedHops: append(
			cleartextPath.Path.BlindedHops,
			blindedPath.BlindedHops...,
		),
	}

	return &sphinx.BlindedPathInfo{
		Path:       combined,
		SessionKey: cleartextPath.SessionKey,
	}, nil
}

// encodeEmptyRouteData encodes an empty BlindedRouteData for use in single-hop
// blinded paths where no forwarding instructions are needed.
func encodeEmptyRouteData() []byte {
	buf, err := record.EncodeBlindedRouteData(
		&record.BlindedRouteData{},
	)
	if err != nil {
		// An empty route data should always encode successfully.
		panic("encode empty route data: " + err.Error())
	}

	return buf
}
