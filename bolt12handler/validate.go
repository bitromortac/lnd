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
	// ErrOfferDisabled is returned when the offer has been administratively
	// disabled.
	ErrOfferDisabled = errors.New("offer is disabled")

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
func CheckArrivalPath(ir *bolt12.InvoiceRequest, signer Signer,
	pathKey *btcec.PublicKey) error {

	if !ir.OfferPaths.IsSome() {
		return nil
	}
	paths := ir.OfferPaths.ValOpt().UnwrapOr(lnwire.BlindedPaths{})

	if pathKey == nil {
		return ErrWrongArrivalPath
	}

	arrivalID, err := blindedNodeID(signer, pathKey)
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

// validateInvoiceRequestForOffer performs the offer-specific validation of an
// invoice request that ValidateInvoiceRequestRead does not cover. It checks
// that the offer is not expired or disabled, and that the offer is priced in
// msat.
//
// The caller must look up the offer by the offer hash of the request. That hit
// proves the offer fields in the request match the stored offer, so the terms
// are read from the request. The caller must also run
// bolt12.ValidateInvoiceRequestRead first, because only the reader checks the
// quantity and the amount.
func validateInvoiceRequestForOffer(ir *bolt12.InvoiceRequest,
	offer *offers.Offer, now uint64) error {

	if offer.IsDisabled {
		return ErrOfferDisabled
	}

	// Offer must not be expired. The offer expires after its last second,
	// as the payer's codec reads offer_absolute_expiry, so a request that
	// the payer could still send in that second is answered.
	if ir.OfferAbsoluteExpiry.IsSome() &&
		now > uint64(ir.OfferAbsoluteExpiry.ValOpt().UnwrapOr(0)) {

		return bolt12.ErrOfferExpired
	}

	// The reader cannot check invreq_amount against an offer_amount in
	// offer_currency units, and the handler has no exchange rate to do it.
	if ir.OfferCurrency.IsSome() {
		return errCurrencyConversion
	}

	return nil
}
