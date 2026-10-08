package bolt12handler

import (
	"errors"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/tlv"
)

var (
	// ErrNotOfferless is returned when an offer-less operation gets a
	// message that answers an offer.
	ErrNotOfferless = errors.New("message answers an offer")

	// ErrNoInvreqPaths is returned when an invoice for a request without
	// invreq_paths arrives. This node always publishes requests with
	// invreq_paths, so such an invoice does not answer one of its
	// requests.
	ErrNoInvreqPaths = errors.New("invoice request has no invreq_paths")

	// ErrWrongInvreqPath is returned when an invoice did not arrive on
	// one of the invreq_paths of the request it answers.
	ErrWrongInvreqPath = errors.New("invoice did not arrive on one of " +
		"the request's paths")
)

// IsOfferless reports whether a message carries no offer identity, so it
// belongs to an invoice request without an offer. An offer always has
// offer_issuer_id or offer_paths.
func IsOfferless(offerIssuerID bool, offerPaths bool) bool {
	return !offerIssuerID && !offerPaths
}

// OfferlessRequestParams holds what a payer puts in an invoice request
// without an offer.
type OfferlessRequestParams struct {
	// Description is the purpose of the payment, set as
	// offer_description.
	Description string

	// AmountMsat is the invreq_amount the request pays. It is mandatory
	// for a request without an offer.
	AmountMsat uint64

	// AbsoluteExpiry, when not zero, is set as offer_absolute_expiry, in
	// seconds since the epoch.
	AbsoluteExpiry uint64

	// Chain is the genesis hash of the chain to pay on. invreq_chain is
	// set only for a chain other than Bitcoin mainnet.
	Chain [32]byte

	// Paths are the blinded paths to this node, set as invreq_paths. The
	// payee sends the invoice along one of them.
	Paths []lnwire.BlindedPath
}

// BuildOfferlessInvoiceRequest builds and signs an invoice request without an
// offer, which a payer publishes as an offer to send money. The payer key and
// the metadata come from the caller's idempotency key, so the request needs
// no stored secret. It always sets invreq_paths: without them the payee would
// send the invoice to invreq_payer_id as a node id, so the payer id would have
// to be this node's routable key.
func BuildOfferlessInvoiceRequest(payer *PayerKey,
	params OfferlessRequestParams) (*bolt12.InvoiceRequest, error) {

	if params.Description == "" {
		return nil, errors.New("description required")
	}
	if params.AmountMsat == 0 {
		return nil, errors.New("amount required")
	}
	if len(params.Paths) == 0 {
		return nil, ErrNoInvreqPaths
	}

	ir := &bolt12.InvoiceRequest{
		OfferDescription: tlv.SomeRecordT(
			tlv.NewPrimitiveRecord[tlv.TlvType10](
				tlv.Blob(params.Description),
			),
		),
		InvreqMetadata: tlv.SomeRecordT(
			tlv.NewPrimitiveRecord[tlv.TlvType0](
				tlv.Blob(payer.Metadata),
			),
		),
		InvreqAmount: tlv.SomeRecordT(
			tlv.NewRecordT[tlv.TlvType82](
				bolt12.TUint64(params.AmountMsat),
			),
		),
		InvreqPayerID: tlv.SomeRecordT(
			tlv.NewPrimitiveRecord[tlv.TlvType88](
				payer.PrivKey.PubKey(),
			),
		),
		InvreqPaths: tlv.SomeRecordT(
			tlv.NewRecordT[tlv.TlvType90](lnwire.BlindedPaths{
				Paths: params.Paths,
			}),
		),
	}

	if params.AbsoluteExpiry != 0 {
		ir.OfferAbsoluteExpiry = tlv.SomeRecordT(
			tlv.NewRecordT[tlv.TlvType14](
				bolt12.TUint64(params.AbsoluteExpiry),
			),
		)
	}

	if params.Chain != bolt12.BitcoinMainnetGenesisHash() {
		ir.InvreqChain = tlv.SomeRecordT(
			tlv.NewPrimitiveRecord[tlv.TlvType80](params.Chain),
		)
	}

	// SignInvoiceRequest runs the writer validation first, so a
	// malformed request fails before the payer key signs it.
	sig, err := bolt12.SignInvoiceRequest(ir, payer.PrivKey)
	if err != nil {
		return nil, fmt.Errorf("sign invoice request: %w", err)
	}
	ir.Signature = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType240](sig),
	)

	return ir, nil
}

// CheckArrivalOnPaths reports whether a message arrived on one of the given
// blinded paths to this node. pathKey is the key the onion message arrived
// under. The final hop of each path names this node by a blinded node id, and
// the node derives the same id from the path key it received only when the
// message came along that path.
func CheckArrivalOnPaths(paths []lnwire.BlindedPath, signer NodeSigner,
	pathKey *btcec.PublicKey) (bool, error) {

	if pathKey == nil {
		return false, nil
	}

	arrivalID, err := signer.BlindedNodePubKey(pathKey)
	if err != nil {
		return false, fmt.Errorf("derive arrival blinded node id: %w",
			err)
	}

	for _, path := range paths {
		if len(path.Hops) == 0 {
			continue
		}

		final := path.Hops[len(path.Hops)-1].BlindedNodeID
		if final != nil && final.IsEqual(arrivalID) {
			return true, nil
		}
	}

	return false, nil
}

// ValidateOfferlessInvoice checks an invoice that answers an invoice request
// without an offer that this node published. The invoice must arrive on one of
// the request's invreq_paths, as the BOLT 12 reader requires, and pass the full
// payer checks against the request. expectedNodeID is the invoice_node_id
// agreed with the payee out of band, or nil when the user approves the
// invoice.
func ValidateOfferlessInvoice(inv *bolt12.Invoice, req *bolt12.InvoiceRequest,
	signer NodeSigner, pathKey *btcec.PublicKey,
	expectedNodeID *btcec.PublicKey, activeChain [32]byte,
	now time.Time) error {

	if !IsOfferless(inv.OfferIssuerID.IsSome(), inv.OfferPaths.IsSome()) {
		return ErrNotOfferless
	}

	paths := req.InvreqPaths.ValOpt().UnwrapOr(lnwire.BlindedPaths{})
	if len(paths.Paths) == 0 {
		return ErrNoInvreqPaths
	}

	ok, err := CheckArrivalOnPaths(paths.Paths, signer, pathKey)
	if err != nil {
		return err
	}
	if !ok {
		return ErrWrongInvreqPath
	}

	return bolt12.ValidateInvoiceForPayment(
		inv, req, now, activeChain, bolt12.InvoiceKnownFeatures{
			Invoice: bolt12.Bolt12Features,
			Blinded: bolt12.Bolt12Features,
		}, expectedNodeID,
	)
}
