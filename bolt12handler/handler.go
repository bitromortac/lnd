// Package bolt12handler implements the BOLT 12 receiver side: handling incoming
// invoice requests, generating signed invoices, and replying via onion
// messages.
package bolt12handler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/invoices"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/offers"
	"github.com/lightningnetwork/lnd/tlv"
)

// InvoiceNotifier sends fire-and-forget notifications for newly generated
// BOLT 12 invoices. Connected SubscribeInvoices callers see the Open-state
// event, but it is not replayable to late-joining subscribers because no
// database row backs it.
type InvoiceNotifier interface {
	// NotifyNewBolt12Invoice notifies connected subscribers about a newly
	// generated BOLT 12 invoice. No database write occurs.
	NotifyNewBolt12Invoice(hash lntypes.Hash,
		invoice *invoices.Invoice)
}

// OnionReplier sends an onion message containing an invoice back to the payer
// via the provided reply path.
type OnionReplier interface {
	// SendInvoiceReply sends the encoded invoice bytes as a type-66 TLV
	// payload via the reply path.
	SendInvoiceReply(ctx context.Context, invoiceBytes []byte,
		replyPath *sphinx.BlindedPath) error
}

// NodeSigner provides the node's identity key for BOLT 12 invoice signing and
// envelope operations.
type NodeSigner interface {
	// NodePubKey returns the node's identity public key.
	NodePubKey() *btcec.PublicKey

	// SignInvoice signs a BOLT 12 invoice using the node's identity private
	// key and returns the 64-byte Schnorr signature.
	SignInvoice(inv *bolt12.Invoice) ([64]byte, error)

	// SignEnvelopeData signs envelope data using a BIP-340 tagged hash:
	// tagged_hash("bolt12/envelope", offerIDHash || data). Returns the
	// 64-byte Schnorr signature.
	SignEnvelopeData(offerIDHash [32]byte,
		data []byte) ([64]byte, error)

	// VerifyEnvelopeData verifies a tagged-hash signature over envelope
	// data using the node's public key.
	VerifyEnvelopeData(offerIDHash [32]byte,
		data []byte, sig [64]byte) error
}

// Handler processes incoming BOLT 12 invoice requests and generates signed
// invoices in response.
type Handler struct {
	offerStore         offers.Store
	notifier           InvoiceNotifier
	replier            OnionReplier
	signer             NodeSigner
	paymentPathBuilder PaymentPathBuilder

	// activeChain is the genesis hash of the chain this handler serves.
	// It gates the spec invreq_chain reader rule.
	activeChain [32]byte
}

// NewHandler creates a new BOLT 12 invoice request handler.
func NewHandler(offerStore offers.Store, notifier InvoiceNotifier,
	replier OnionReplier, signer NodeSigner,
	paymentPathBuilder PaymentPathBuilder,
	activeChain [32]byte) *Handler {

	return &Handler{
		offerStore:         offerStore,
		notifier:           notifier,
		replier:            replier,
		signer:             signer,
		paymentPathBuilder: paymentPathBuilder,
		activeChain:        activeChain,
	}
}

// SetPaymentPathBuilder sets the payment path builder for multi-hop blinded
// payment paths. This is called after the router is initialized.
func (h *Handler) SetPaymentPathBuilder(b PaymentPathBuilder) {
	h.paymentPathBuilder = b
}

// HandleInvoiceRequest is the top-level entry point called when an onion
// message with TLV type 64 (invoice request) arrives.
func (h *Handler) HandleInvoiceRequest(ctx context.Context, invreqBytes []byte,
	replyPath *sphinx.BlindedPath) error {

	// Decode the raw bytes as a BOLT 12 invoice request.
	ir, err := bolt12.DecodeInvoiceRequest(invreqBytes)
	if err != nil {
		return fmt.Errorf("decode invoice request: %w", err)
	}

	// Run generic structural and signature validation.
	if err := bolt12.ValidateInvoiceRequestRead(
		ir, h.activeChain,
	); err != nil {
		return fmt.Errorf("validate invoice request: %w", err)
	}

	// Look up the matching offer.
	offer, err := h.lookupOffer(ctx, ir)
	if err != nil {
		return fmt.Errorf("lookup offer: %w", err)
	}

	// Validate the request against the offer.
	now := uint64(time.Now().Unix())
	if err := ValidateInvoiceRequestForOffer(
		ir, offer, now,
	); err != nil {

		return fmt.Errorf("validate against offer: %w", err)
	}

	// Generate the invoice with the offer ID hash for envelope signing.
	result, err := GenerateInvoice(
		ir, h.signer, h.paymentPathBuilder, offer.OfferID,
	)
	if err != nil {
		return fmt.Errorf("generate invoice: %w", err)
	}

	// Notify connected subscribers about the new invoice. This is
	// fire-and-forget: no database write occurs. The invoice will be
	// reconstructed from the signed envelope at HTLC settlement time.
	h.notifyInvoice(result, offer)

	// Auto-reply with the signed invoice via the reply path, if one was
	// provided.
	if replyPath != nil {
		invoiceBytes, encErr := result.Invoice.Encode()
		if encErr != nil {
			return fmt.Errorf("encode reply: %w", encErr)
		}

		if err := h.replier.SendInvoiceReply(
			ctx, invoiceBytes, replyPath,
		); err != nil {

			return fmt.Errorf("send reply: %w", err)
		}
	}

	return nil
}

// lookupOffer finds the stored offer that matches the invoice request's offer
// fields. It computes the offer_id from the offer fields in the request and
// looks it up in the store.
func (h *Handler) lookupOffer(ctx context.Context, ir *bolt12.InvoiceRequest) (
	*offers.Offer, error) {

	// Re-encode just the offer fields (types 2-22) from the invoice request
	// to compute the offer_id hash.
	offerFromIR := &bolt12.Offer{
		OfferChains:         ir.OfferChains,
		OfferMetadata:       ir.OfferMetadata,
		OfferCurrency:       ir.OfferCurrency,
		OfferAmount:         ir.OfferAmount,
		OfferDescription:    ir.OfferDescription,
		OfferFeatures:       ir.OfferFeatures,
		OfferAbsoluteExpiry: ir.OfferAbsoluteExpiry,
		OfferPaths:          ir.OfferPaths,
		OfferIssuer:         ir.OfferIssuer,
		OfferQuantityMax:    ir.OfferQuantityMax,
		OfferIssuerID:       ir.OfferIssuerID,
	}

	tlvBytes, err := offerFromIR.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode offer fields: %w", err)
	}

	offerID := sha256.Sum256(tlvBytes)

	return h.offerStore.GetOfferByOfferID(ctx, offerID)
}

// notifyInvoice sends a fire-and-forget notification about a newly generated
// BOLT 12 invoice to connected subscribers. No database write occurs — the
// invoice will be reconstructed from the signed envelope when the HTLC arrives.
func (h *Handler) notifyInvoice(result *InvoiceResult,
	offer *offers.Offer) {

	// Extract invreq_payer_id as the serialised compressed point, for the
	// invoices.Invoice struct which stores it as []byte.
	var payerIDBytes []byte
	result.Invoice.InvreqPayerID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType88, *btcec.PublicKey]) {
			payerIDBytes = r.Val.SerializeCompressed()
		},
	)

	invoice := &invoices.Invoice{
		CreationDate: time.Now().UTC(),
		Terms: invoices.ContractTerm{
			Expiry:          7200 * time.Second,
			PaymentPreimage: &result.Preimage,
			PaymentAddr:     result.PathID,
			Value: lnwire.MilliSatoshi(
				getInvoiceAmount(result.Invoice),
			),
			Features: lnwire.EmptyFeatureVector(),
		},
		PaymentRequest: []byte(result.Encoded),
		IsBolt12:       true,
		OfferID:        &offer.ID,
		InvoiceNodeID: h.signer.NodePubKey().
			SerializeCompressed(),
		InvreqPayerID: payerIDBytes,
	}

	if h.notifier != nil {
		h.notifier.NotifyNewBolt12Invoice(
			result.PaymentHash, invoice,
		)
	}
}

// getInvoiceAmount extracts the invoice_amount from a bolt12.Invoice, returning
// the raw uint64 value.
func getInvoiceAmount(inv *bolt12.Invoice) uint64 {
	var amt uint64
	inv.InvoiceAmount.WhenSome(
		func(r tlv.RecordT[tlv.TlvType170, bolt12.TUint64]) {
			amt = uint64(r.Val)
		},
	)

	return amt
}
