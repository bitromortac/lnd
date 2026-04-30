package bolt12handler

import (
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/offers"
)

var (
	// ErrOfferExpired is returned when the referenced offer has expired.
	ErrOfferExpired = errors.New("offer has expired")

	// ErrOfferDisabled is returned when the offer has been administratively
	// disabled.
	ErrOfferDisabled = errors.New("offer is disabled")

	// ErrAmountBelowExpected is returned when invreq_amount is below the
	// expected amount.
	ErrAmountBelowExpected = errors.New("invreq_amount below expected " +
		"amount")

	// ErrMissingInvreqAmount is returned when the offer has no fixed amount
	// and invreq_amount is absent.
	ErrMissingInvreqAmount = errors.New("invreq_amount required when " +
		"offer has no amount")

	// ErrQuantityNotExpected is returned when invreq_quantity is present
	// but the offer does not support quantity.
	ErrQuantityNotExpected = errors.New("invreq_quantity present but " +
		"offer has no quantity_max")

	// ErrMissingQuantity is returned when invreq_quantity is absent but the
	// offer requires it.
	ErrMissingQuantity = errors.New("invreq_quantity required when offer " +
		"has quantity_max")

	// ErrWrongArrivalPath is returned when an invoice request for an offer
	// with offer_paths did not arrive on one of them. The reader must
	// ignore such a request, so a node never reveals that it also created
	// another offer.
	ErrWrongArrivalPath = errors.New("invoice request did not arrive " +
		"on one of the offer's paths")
)

// CheckArrivalPath enforces the reader rule that an invoice request for an
// offer with offer_paths must arrive on one of those paths. pathKey is the key
// the onion message arrived under at this node.
//
// The check needs no state. The final hop of each offer path names this node
// by a blinded node id, and the node derives the same id from the path key it
// received only when the message came along that path. Any other path, also
// one the node made for a different offer, gives a different id.
//
// The rule for an offer without offer_paths, that the request must not arrive
// on a blinded path the node made, needs the path_id of the arrival path, and
// the onion message layer does not pass it on yet.
func CheckArrivalPath(ir *bolt12.InvoiceRequest, signer NodeSigner,
	pathKey *btcec.PublicKey) error {

	if !ir.OfferPaths.IsSome() {
		return nil
	}
	paths := ir.OfferPaths.ValOpt().UnwrapOr(lnwire.BlindedPaths{})

	if pathKey == nil {
		return ErrWrongArrivalPath
	}

	arrivalID, err := signer.BlindedNodePubKey(pathKey)
	if err != nil {
		return fmt.Errorf("derive arrival blinded node id: %w", err)
	}

	for _, path := range paths.Paths {
		if len(path.Hops) == 0 {
			continue
		}

		final := path.Hops[len(path.Hops)-1].BlindedNodeID
		if final != nil && final.IsEqual(arrivalID) {
			return nil
		}
	}

	return ErrWrongArrivalPath
}

// ValidateInvoiceRequestForOffer performs the offer-specific validation of an
// invoice request that ValidateInvoiceRequestRead does not cover. It checks
// that the offer is not expired or disabled, and that the amount and quantity
// constraints are satisfied.
//
// The caller must look up the offer by the offer hash of the request. That hit
// proves the offer fields in the request match the stored offer, so the terms
// are read from the request. The caller must also run
// bolt12.ValidateInvoiceRequestRead first for the generic structural and
// signature checks.
func ValidateInvoiceRequestForOffer(ir *bolt12.InvoiceRequest,
	offer *offers.Offer, now uint64) error {

	// Offer must not be disabled.
	if offer.IsDisabled {
		return ErrOfferDisabled
	}

	// Offer must not be expired. The offer expires after its last second,
	// as the payer's codec reads offer_absolute_expiry, so a request that
	// the payer could still send in that second is answered.
	if ir.OfferAbsoluteExpiry.IsSome() &&
		now > uint64(ir.OfferAbsoluteExpiry.ValOpt().UnwrapOr(0)) {

		return ErrOfferExpired
	}

	// lnd never issues an offer priced in a currency, and the expected
	// amount below is in msat. Refuse such a request explicitly rather
	// than compare a currency amount with msat.
	if ir.OfferCurrency.IsSome() {
		return ErrCurrencyNotSupported
	}

	// Validate quantity constraints.
	hasInvreqQty := ir.InvreqQuantity.IsSome()
	if ir.OfferQuantityMax.IsSome() {
		if !hasInvreqQty {
			return ErrMissingQuantity
		}

		// Quantity bounds are already checked by
		// ValidateInvoiceRequestRead, so we skip re-checking here.
	} else {
		if hasInvreqQty {
			return ErrQuantityNotExpected
		}
	}

	// Validate amount constraints. When the offer has no fixed amount,
	// invreq_amount is mandatory.
	if !ir.OfferAmount.IsSome() && !ir.InvreqAmount.IsSome() {
		return ErrMissingInvreqAmount
	}

	expectedAmount, err := expectedOfferAmount(
		uint64(ir.OfferAmount.ValOpt().UnwrapOr(0)),
		uint64(ir.InvreqQuantity.ValOpt().UnwrapOr(0)),
	)
	if err != nil {
		return err
	}

	// TODO: Should we reject an invreq_amount far above the expected
	// amount, for example more than twice, as BOLT 4 recommends for
	// payments? A BOLT 11 invoice has no upper limit when a payment
	// settles it with a higher amount, and the intent to overpay is the
	// same.
	if ir.InvreqAmount.IsSome() {
		invreqAmt := uint64(ir.InvreqAmount.ValOpt().UnwrapOr(0))
		if invreqAmt < expectedAmount {
			return fmt.Errorf("%w: got %d, expected >= %d",
				ErrAmountBelowExpected, invreqAmt,
				expectedAmount)
		}
	}

	return nil
}
