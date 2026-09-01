package bolt12handler

import (
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/offers"
	"github.com/lightningnetwork/lnd/tlv"
)

var (
	// ErrOfferExpired is returned when the referenced offer has expired.
	ErrOfferExpired = errors.New("offer has expired")

	// ErrOfferDisabled is returned when the offer has been administratively
	// disabled.
	ErrOfferDisabled = errors.New("offer is disabled")

	// ErrOfferFieldMismatch is returned when the offer fields in the
	// invoice request do not match the stored offer.
	ErrOfferFieldMismatch = errors.New("offer fields do not match stored " +
		"offer")

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
// that the offer fields match, the offer is not expired or disabled, and that
// the amount and quantity constraints are satisfied.
//
// The caller must run bolt12.ValidateInvoiceRequestRead first for the generic
// structural and signature checks.
func ValidateInvoiceRequestForOffer(ir *bolt12.InvoiceRequest,
	offer *offers.Offer, now uint64) error {

	// Offer must not be disabled.
	if offer.IsDisabled {
		return ErrOfferDisabled
	}

	// Offer must not be expired.
	if offer.HasExpiry && now > offer.AbsoluteExpiry {
		return ErrOfferExpired
	}

	// Verify that the offer fields in the invoice request match the stored
	// offer by re-encoding the stored offer and comparing the offer TLV
	// fields from the request.
	if err := matchOfferFields(ir, offer); err != nil {
		return err
	}

	// Validate quantity constraints.
	hasInvreqQty := hasOptField(ir.InvreqQuantity)
	if offer.HasQuantityMax {
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

	// Validate amount constraints.
	if offer.HasAmount {
		expectedAmount := offer.AmountMsat
		if hasInvreqQty {
			qty := getUint64Field(ir.InvreqQuantity)
			expectedAmount *= qty
		}

		if hasOptField(ir.InvreqAmount) {
			invreqAmt := getUint64Field(ir.InvreqAmount)
			if invreqAmt < expectedAmount {
				return fmt.Errorf("%w: got %d, expected >= %d",
					ErrAmountBelowExpected, invreqAmt,
					expectedAmount)
			}
		}
	} else {
		// No offer_amount — invreq_amount is mandatory.
		if !hasOptField(ir.InvreqAmount) {
			return ErrMissingInvreqAmount
		}
	}

	return nil
}

// matchOfferFields verifies that the offer fields embedded in the invoice
// request match the stored offer. The spec requires exact matching of the offer
// fields.
func matchOfferFields(ir *bolt12.InvoiceRequest, offer *offers.Offer) error {

	// Compare offer_issuer_id from the request against the stored offer's
	// issuer node ID. Both may be absent when offer_paths is used.
	var (
		issuerID         [33]byte
		requestHasIssuer bool
	)
	ir.OfferIssuerID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType22, *btcec.PublicKey]) {
			copy(issuerID[:], r.Val.SerializeCompressed())
			requestHasIssuer = true
		},
	)

	emptyIssuer := [33]byte{}
	storedHasIssuer := offer.IssuerNodeID != emptyIssuer

	if storedHasIssuer || requestHasIssuer {
		if issuerID != offer.IssuerNodeID {
			return fmt.Errorf("%w: offer_issuer_id mismatch",
				ErrOfferFieldMismatch)
		}
	}

	// Compare offer_description.
	var descBytes []byte
	ir.OfferDescription.WhenSome(
		func(r tlv.RecordT[tlv.TlvType10, tlv.Blob]) {
			descBytes = r.Val
		},
	)

	if string(descBytes) != offer.Description {
		return fmt.Errorf("%w: offer_description mismatch",
			ErrOfferFieldMismatch)
	}

	// Compare offer_amount.
	if offer.HasAmount {
		irAmt := getUint64Field(ir.OfferAmount)
		if irAmt != offer.AmountMsat {
			return fmt.Errorf("%w: offer_amount mismatch",
				ErrOfferFieldMismatch)
		}
	}

	return nil
}

// hasOptField returns true if the optional record is set.
func hasOptField[T tlv.TlvType, V any](opt tlv.OptionalRecordT[T, V]) bool {

	set := false
	opt.WhenSome(func(_ tlv.RecordT[T, V]) {
		set = true
	})

	return set
}

// getUint64Field extracts the uint64 value from an optional TUint64 record,
// returning 0 if absent.
func getUint64Field[T tlv.TlvType](opt tlv.OptionalRecordT[T,
	bolt12.TUint64]) uint64 {

	var val uint64
	opt.WhenSome(
		func(r tlv.RecordT[T, bolt12.TUint64]) {
			val = uint64(r.Val)
		},
	)

	return val
}
