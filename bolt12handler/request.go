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
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/onionmessage"
	"github.com/lightningnetwork/lnd/record"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/subscribe"
	"github.com/lightningnetwork/lnd/tlv"
)

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

	ir := &bolt12.InvoiceRequest{}

	// Mirror all offer fields (types 2-22).
	ir.OfferChains = offer.OfferChains
	ir.OfferMetadata = offer.OfferMetadata
	ir.OfferCurrency = offer.OfferCurrency
	ir.OfferAmount = offer.OfferAmount
	ir.OfferDescription = offer.OfferDescription
	ir.OfferFeatures = offer.OfferFeatures
	ir.OfferAbsoluteExpiry = offer.OfferAbsoluteExpiry
	ir.OfferPaths = offer.OfferPaths
	ir.OfferIssuer = offer.OfferIssuer
	ir.OfferQuantityMax = offer.OfferQuantityMax
	ir.OfferIssuerID = offer.OfferIssuerID

	metadata := make([]byte, 32)
	if _, err := rand.Read(metadata); err != nil {
		return nil, nil, fmt.Errorf("generate metadata: %w", err)
	}
	ir.InvreqMetadata = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType0, tlv.Blob]{
			Val: metadata,
		},
	)

	ir.InvreqPayerID = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType88](payerKey.PubKey()),
	)

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

	// Set invreq_chain (type 80) if the offer specifies a non-mainnet
	// chain. Per spec, this defaults to Bitcoin mainnet when absent, so
	// it must be explicitly set for regtest/testnet/signet.
	offer.OfferChains.WhenSome(
		func(r tlv.RecordT[tlv.TlvType2, bolt12.ChainsRecord]) {
			if len(r.Val.Chains) > 0 {
				chain := r.Val.Chains[0]
				ir.InvreqChain = tlv.SomeRecordT(
					tlv.NewPrimitiveRecord[
						tlv.TlvType80, [32]byte,
					](chain),
				)
			}
		},
	)

	// Encode → decode round-trip to populate rawTLVs for signing.
	irBytes, err := ir.Encode()
	if err != nil {
		return nil, nil, fmt.Errorf("encode invreq: %w", err)
	}

	ir, err = bolt12.DecodeInvoiceRequest(irBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("re-decode invreq: %w", err)
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
// message. It builds a single-hop blinded path to the recipient, wraps the
// request in a type-64 TLV payload, and sends it with the provided reply path.
func SendInvoiceRequest(ctx context.Context, invreqBytes []byte,
	recipientPubKey *btcec.PublicKey, replyPath *sphinx.BlindedPathInfo,
	sender OnionMessageSender) error {

	recipientSessionKey, err := btcec.NewPrivateKey()
	if err != nil {
		return fmt.Errorf("generate recipient session key: %w", err)
	}

	recipientHops := []*sphinx.HopInfo{
		{
			NodePub:   recipientPubKey,
			PlainText: encodeEmptyRouteData(),
		},
	}

	recipientPath, err := sphinx.BuildBlindedPath(
		recipientSessionKey, recipientHops,
	)
	if err != nil {
		return fmt.Errorf("build recipient path: %w", err)
	}

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
		recipientPath.Path,
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
			sphinx.MaxRoutingPayloadSize,
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
		peerPub[:], recipientPath.Path.IntroductionPoint.
			SerializeCompressed(),
	)

	return sender.SendOnionMessage(
		ctx, peerPub, recipientPath.Path.BlindingPoint, buf.Bytes(),
	)
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
