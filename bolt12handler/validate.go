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

	// Offer must not be expired. The receiver treats the offer as expired
	// from the offer_absolute_expiry second onward.
	if hasOptField(ir.OfferAbsoluteExpiry) &&
		now >= getUint64Field(ir.OfferAbsoluteExpiry) {

		return ErrOfferExpired
	}

	// Validate quantity constraints.
	hasInvreqQty := hasOptField(ir.InvreqQuantity)
	if hasOptField(ir.OfferQuantityMax) {
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
	if !hasOptField(ir.OfferAmount) && !hasOptField(ir.InvreqAmount) {
		return ErrMissingInvreqAmount
	}

	expectedAmount := getUint64Field(ir.OfferAmount)
	if hasInvreqQty {
		expectedAmount *= getUint64Field(ir.InvreqQuantity)
	}

	if hasOptField(ir.InvreqAmount) {
		invreqAmt := getUint64Field(ir.InvreqAmount)
		if invreqAmt < expectedAmount {
			return fmt.Errorf("%w: got %d, expected >= %d",
				ErrAmountBelowExpected, invreqAmt,
				expectedAmount)
		}
	}

	return nil
}
