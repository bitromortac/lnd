package bolt12handler

import (
	"errors"
	"fmt"

	"github.com/lightningnetwork/lnd/bolt12"
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
)

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

	expectedAmount := uint64(ir.OfferAmount.ValOpt().UnwrapOr(0))
	if hasInvreqQty {
		expectedAmount *= uint64(ir.InvreqQuantity.ValOpt().UnwrapOr(0))
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
