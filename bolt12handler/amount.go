package bolt12handler

import (
	"errors"

	"github.com/lightningnetwork/lnd/bolt12"
)

var (
	// ErrCurrencyNotSupported is returned for an invoice request whose
	// offer is priced in a currency. lnd issues no such offers, and it
	// has no exchange rate to convert the amount.
	ErrCurrencyNotSupported = errors.New("offers priced in a currency " +
		"are not supported")

	// ErrCurrencyNeedsAmount is returned when the payer pays an offer
	// priced in a currency without an explicit amount. lnd cannot convert
	// the currency amount, so the caller must state the msat amount.
	ErrCurrencyNeedsAmount = errors.New("the offer is priced in a " +
		"currency: set the amount in msat")

	// ErrPayOfferAmountRequired is returned when the payer pays an offer
	// without a fixed amount and gives no amount.
	ErrPayOfferAmountRequired = errors.New("the offer has no amount: " +
		"set the amount in msat")
)

// PayOfferAmount returns the invreq_amount of an offer payment. The payer
// always sets invreq_amount, because the payer then requires invoice_amount to
// equal it, and the payee cannot ask for more than the payer authorized.
//
// An explicit amount is used as it is, and the request writer checks that it
// covers offer_amount times the quantity. Without one, the amount is the
// offer's own. An offer priced in a currency needs an explicit amount, because
// the node has no exchange rate.
func PayOfferAmount(offer *bolt12.Offer, amountMsat,
	quantity uint64) (uint64, error) {

	if offer.OfferCurrency.IsSome() {
		if amountMsat == 0 {
			return 0, ErrCurrencyNeedsAmount
		}

		return amountMsat, nil
	}

	if amountMsat > 0 {
		return amountMsat, nil
	}

	if !offer.OfferAmount.IsSome() {
		return 0, ErrPayOfferAmountRequired
	}

	return expectedOfferAmount(
		uint64(offer.OfferAmount.ValOpt().UnwrapOr(0)), quantity,
	)
}
