package lnd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/bolt12handler"
	"github.com/lightningnetwork/lnd/invoices"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/onionmessage"
	paymentsdb "github.com/lightningnetwork/lnd/payments/db"
	"github.com/lightningnetwork/lnd/tlv"
)

const (
	// maxPendingPerRequest bounds the invoices that wait for approval for
	// one published request. Anyone who saw the request can answer it, so
	// the bound keeps a flood of invoices from growing memory.
	maxPendingPerRequest = 5

	// maxPendingInvoices bounds the invoices that wait for approval over
	// all published requests.
	maxPendingInvoices = 100

	// offerlessPaymentTimeout is how long the node tries to pay an invoice
	// for one of its published requests.
	offerlessPaymentTimeout = 60 * time.Second
)

var (
	// errPendingInvoiceNotFound is returned when an approval names no
	// invoice that waits for approval.
	errPendingInvoiceNotFound = errors.New("no invoice waits for " +
		"approval with this payment hash")

	// errInvoiceRequestNotOpen is returned when an invoice answers a
	// published request that expired or already has a payment.
	errInvoiceRequestNotOpen = errors.New("invoice request is expired or " +
		"already paid")
)

// pendingInvoice is an invoice for one of this node's published requests
// that waits for the user's approval, because the request names no expected
// invoice_node_id. It lives in memory only, so an unsolicited invoice causes
// no database write.
type pendingInvoice struct {
	// requestID is the database ID of the published request.
	requestID int64

	// invoice is the validated invoice.
	invoice *bolt12.Invoice

	// encoded is the invoice as an lni1 string.
	encoded string

	// paymentHash is the invoice's payment hash.
	paymentHash lntypes.Hash

	// nodeID is the invoice_node_id that signed the invoice.
	nodeID *btcec.PublicKey
}

// offerlessPayer pays the invoices that answer the invoice requests without
// an offer that this node published. An invoice from the expected node is paid
// at once. Any other valid invoice waits in memory for the user's approval.
type offerlessPayer struct {
	store    paymentsdb.Bolt12InvoiceRequestStore
	signer   *bolt12handler.KeyRingSigner
	chain    [32]byte
	payments interface {
		FetchPayment(ctx context.Context,
			hash lntypes.Hash) (*paymentsdb.MPPayment, error)
	}

	// pay pays the invoice for the request and returns the preimage.
	pay func(ctx context.Context, req *paymentsdb.Bolt12InvoiceRequest,
		inv *bolt12.Invoice, feeLimitMsat int64) (lntypes.Preimage,
		error)

	mu      sync.Mutex
	pending map[lntypes.Hash]*pendingInvoice
}

// requestOpen reports whether a published request can still take a payment:
// it has not expired, and it has no payment or only a failed one. The payments
// store checks this again in the transaction that starts the payment.
func (p *offerlessPayer) requestOpen(ctx context.Context,
	req *paymentsdb.Bolt12InvoiceRequest, now time.Time) bool {

	if req.Expired(now) {
		return false
	}

	if !req.Used {
		return true
	}

	if req.PaymentHash == nil {
		return false
	}

	payment, err := p.payments.FetchPayment(ctx, *req.PaymentHash)
	if err != nil {
		return false
	}

	return payment.GetStatus() == paymentsdb.StatusFailed
}

// handleInvoice processes an invoice that arrived in an onion message under
// pathKey. It ignores an invoice for an offer, which PayOffer handles, and any
// invoice that does not answer one of this node's open requests.
func (p *offerlessPayer) handleInvoice(ctx context.Context,
	invoiceBytes []byte, pathKey *btcec.PublicKey) error {

	inv, err := bolt12.DecodeInvoice(invoiceBytes)
	if err != nil {
		return fmt.Errorf("decode invoice: %w", err)
	}

	if !bolt12handler.IsOfferless(
		inv.OfferIssuerID.IsSome(), inv.OfferPaths.IsSome(),
	) {

		return nil
	}

	metadata := inv.InvreqMetadata.ValOpt().UnwrapOr(nil)
	if len(metadata) == 0 {
		return nil
	}

	req, err := p.store.FetchBolt12InvoiceRequestByMetadata(
		ctx, metadata,
	)
	if errors.Is(err, paymentsdb.ErrBolt12InvoiceRequestNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	now := time.Now()
	if !p.requestOpen(ctx, req, now) {
		return errInvoiceRequestNotOpen
	}

	ir, err := bolt12.DecodeInvoiceRequestString(req.Encoded, p.chain)
	if err != nil {
		return fmt.Errorf("decode stored invoice request: %w", err)
	}

	var expected *btcec.PublicKey
	if len(req.ExpectedNodeID) > 0 {
		expected, err = btcec.ParsePubKey(req.ExpectedNodeID)
		if err != nil {
			return fmt.Errorf("parse expected node id: %w", err)
		}
	}

	err = bolt12handler.ValidateOfferlessInvoice(
		inv, ir, p.signer, pathKey, expected, p.chain, now,
	)
	if err != nil {
		return fmt.Errorf("validate invoice: %w", err)
	}

	// An invoice from the agreed node is paid at once. The payment can
	// take long, so it runs on its own.
	if expected != nil {
		go func() {
			ctx, cancel := context.WithTimeout(
				context.Background(), offerlessPaymentTimeout,
			)
			defer cancel()

			_, err := p.pay(ctx, req, inv, int64(req.FeeLimit))
			if err != nil {
				srvrLog.Warnf("Paying invoice for BOLT 12 "+
					"invoice request %d failed: %v",
					req.ID, err)
			}
		}()

		return nil
	}

	return p.addPending(req.ID, inv)
}

// addPending keeps a validated invoice in memory until the user approves it.
func (p *offerlessPayer) addPending(requestID int64,
	inv *bolt12.Invoice) error {

	encoded, err := bolt12.EncodeInvoiceString(inv)
	if err != nil {
		return fmt.Errorf("encode invoice: %w", err)
	}

	hash := lntypes.Hash(inv.InvoicePaymentHash.ValOpt().UnwrapOr(
		[32]byte{},
	))
	nodeID := inv.InvoiceNodeID.ValOpt().UnwrapOr(nil)

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.pending[hash]; ok {
		return nil
	}

	perRequest := 0
	for _, pi := range p.pending {
		if pi.requestID == requestID {
			perRequest++
		}
	}
	if perRequest >= maxPendingPerRequest ||
		len(p.pending) >= maxPendingInvoices {

		return errors.New("too many invoices wait for approval")
	}

	p.pending[hash] = &pendingInvoice{
		requestID:   requestID,
		invoice:     inv,
		encoded:     encoded,
		paymentHash: hash,
		nodeID:      nodeID,
	}

	srvrLog.Infof("BOLT 12 invoice %v for invoice request %d waits "+
		"for approval", hash, requestID)

	return nil
}

// pendingFor returns the invoices that wait for approval for a request.
func (p *offerlessPayer) pendingFor(requestID int64) []*pendingInvoice {
	p.mu.Lock()
	defer p.mu.Unlock()

	var out []*pendingInvoice
	for _, pi := range p.pending {
		if pi.requestID == requestID {
			out = append(out, pi)
		}
	}

	return out
}

// approve pays an invoice that waits for approval. The request is checked
// again, because another invoice may have paid it in the meantime.
func (p *offerlessPayer) approve(ctx context.Context, hash lntypes.Hash,
	feeLimitMsat int64) (lntypes.Preimage, error) {

	p.mu.Lock()
	pi, ok := p.pending[hash]
	p.mu.Unlock()
	if !ok {
		return lntypes.Preimage{}, errPendingInvoiceNotFound
	}

	metadata := pi.invoice.InvreqMetadata.ValOpt().UnwrapOr(nil)
	req, err := p.store.FetchBolt12InvoiceRequestByMetadata(
		ctx, metadata,
	)
	if err != nil {
		return lntypes.Preimage{}, err
	}

	if !p.requestOpen(ctx, req, time.Now()) {
		p.dropRequest(req.ID)

		return lntypes.Preimage{}, errInvoiceRequestNotOpen
	}

	preimage, err := p.pay(ctx, req, pi.invoice, feeLimitMsat)
	if err != nil {
		return lntypes.Preimage{}, err
	}

	// The request paid once, so its other invoices can never be paid.
	p.dropRequest(req.ID)

	return preimage, nil
}

// dropRequest forgets every invoice that waits for approval for a request.
func (p *offerlessPayer) dropRequest(requestID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for hash, pi := range p.pending {
		if pi.requestID == requestID {
			delete(p.pending, hash)
		}
	}
}

// payOfferlessInvoice pays an invoice that answers one of this node's
// published requests. The payments store binds the payment to the request in
// the transaction that creates it, so the request pays at most once.
func (s *server) payOfferlessInvoice(ctx context.Context,
	req *paymentsdb.Bolt12InvoiceRequest, inv *bolt12.Invoice,
	feeLimitMsat int64) (lntypes.Preimage, error) {

	pathSet, err := bolt12handler.Bolt12InvoiceToBlindedPathSet(
		inv, s.sciddirResolver,
	)
	if err != nil {
		return lntypes.Preimage{}, fmt.Errorf("convert blinded "+
			"paths: %w", err)
	}

	encoded, err := bolt12.EncodeInvoiceString(inv)
	if err != nil {
		return lntypes.Preimage{}, fmt.Errorf("encode invoice: %w",
			err)
	}

	payment, err := bolt12handler.BuildLightningPayment(
		inv, pathSet, encoded, nil, feeLimitMsat,
		uint64(offerlessPaymentTimeout/time.Second),
	)
	if err != nil {
		return lntypes.Preimage{}, fmt.Errorf("build payment: %w",
			err)
	}
	payment.Bolt12Request = &paymentsdb.Bolt12RequestBinding{
		RequestID: req.ID,
	}

	preimage, _, err := s.chanRouter.SendPayment(ctx, payment)
	if err != nil {
		return lntypes.Preimage{}, fmt.Errorf("payment failed: %w",
			err)
	}

	return lntypes.Preimage(preimage), nil
}

// sendOfferlessInvoice answers an invoice request without an offer as the
// payee. The operator starts this flow, so the node stores the invoice at
// once, like a BOLT 11 invoice, and settlement finds it by payment hash. The
// invoice goes to the request's first invreq_path, or to invreq_payer_id as a
// node id when the request has no paths.
func (s *server) sendOfferlessInvoice(ctx context.Context,
	lnr string) (*bolt12handler.InvoiceResult, error) {

	chain := *s.cfg.ActiveNetParams.GenesisHash
	ir, err := bolt12.DecodeInvoiceRequestString(lnr, chain)
	if err != nil {
		return nil, fmt.Errorf("decode invoice request: %w", err)
	}

	if !bolt12handler.IsOfferless(
		ir.OfferIssuerID.IsSome(), ir.OfferPaths.IsSome(),
	) {

		return nil, errors.New("the invoice request answers an " +
			"offer, so it is not a request to be paid")
	}

	now := time.Now()
	expiry := ir.OfferAbsoluteExpiry.ValOpt().UnwrapOr(0)
	if expiry != 0 && uint64(now.Unix()) > uint64(expiry) {
		return nil, errors.New("the invoice request has expired")
	}

	result, err := s.bolt12Handler.GenerateOfferlessInvoice(ir)
	if err != nil {
		return nil, fmt.Errorf("generate invoice: %w", err)
	}

	preimage := result.Preimage
	invoice := &invoices.Invoice{
		CreationDate: now,
		Terms: invoices.ContractTerm{
			Expiry:          bolt12.DefaultInvoiceRelativeExpiry,
			FinalCltvDelta:  bolt12handler.FinalCLTVDelta,
			PaymentPreimage: &preimage,
			PaymentAddr:     result.PathID,
			Value: lnwire.MilliSatoshi(
				result.Invoice.InvoiceAmount.ValOpt().
					UnwrapOr(0),
			),
			Features: lnwire.EmptyFeatureVector(),
		},
		PaymentRequest: []byte(result.Encoded),
		IsBolt12:       true,
	}

	description := ir.OfferDescription.ValOpt().UnwrapOr(nil)
	if len(description) <= invoices.MaxMemoSize {
		invoice.Memo = description
	}

	ir.InvreqPayerID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType88, *btcec.PublicKey]) {
			invoice.InvreqPayerID = r.Val.SerializeCompressed()
		},
	)

	if _, err := s.invoices.AddInvoice(
		ctx, invoice, result.PaymentHash,
	); err != nil {
		return nil, fmt.Errorf("store invoice: %w", err)
	}

	invoiceBytes, err := result.Invoice.EncodeSigned()
	if err != nil {
		return nil, fmt.Errorf("encode invoice: %w", err)
	}

	paths := ir.InvreqPaths.ValOpt().UnwrapOr(lnwire.BlindedPaths{})
	if len(paths.Paths) > 0 {
		path, err := paths.Paths[0].ToSphinx(s.sciddirResolver)
		if err != nil {
			return nil, fmt.Errorf("resolve invoice request "+
				"path: %w", err)
		}

		err = s.bolt12Replier.SendInvoiceReply(ctx, invoiceBytes, path)
		if err != nil {
			return nil, fmt.Errorf("send invoice: %w", err)
		}

		return result, nil
	}

	payerID := ir.InvreqPayerID.ValOpt().UnwrapOr(nil)
	if payerID == nil {
		return nil, errors.New("the invoice request has no payer id")
	}

	direct, err := bolt12handler.BuildSingleHopReplyPath(payerID)
	if err != nil {
		return nil, fmt.Errorf("build path to payer: %w", err)
	}

	err = s.bolt12Replier.SendInvoiceReply(ctx, invoiceBytes, direct.Path)
	if err != nil {
		return nil, fmt.Errorf("send invoice: %w", err)
	}

	return result, nil
}

// bolt12OfferlessInvoiceLoop passes every invoice that arrives in an onion
// message to the offer-less payer. PayOffer handles the invoices for offers.
func (s *server) bolt12OfferlessInvoiceLoop() {
	defer s.wg.Done()

	client, err := s.onionMessageServer.Subscribe()
	if err != nil {
		srvrLog.Errorf("Failed to subscribe to onion messages for "+
			"BOLT 12 invoices: %v", err)

		return
	}
	defer client.Cancel()

	for {
		select {
		case update, ok := <-client.Updates():
			if !ok {
				return
			}

			msg, ok := update.(*onionmessage.OnionMessageUpdate)
			if !ok {
				continue
			}

			invoiceBytes, ok := msg.CustomRecords[uint64(
				lnwire.InvoiceNamespaceType,
			)]
			if !ok {
				continue
			}

			pathKey, err := btcec.ParsePubKey(msg.PathKey[:])
			if err != nil {
				continue
			}

			err = s.offerlessPayer.handleInvoice(
				context.Background(), invoiceBytes, pathKey,
			)
			if err != nil {
				srvrLog.Debugf("Ignoring BOLT 12 invoice: %v",
					err)
			}

		case <-s.quit:
			return
		}
	}
}
