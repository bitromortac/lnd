package bolt12handler

import (
	"math"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/offers"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// TestValidateInvoiceRequestForOffer_Currency verifies that a request for an
// offer priced in a currency is refused, as lnd issues no such offers.
func TestValidateInvoiceRequestForOffer_Currency(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	offer := testOffer(t, key, 1000)
	offer.OfferCurrency = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType6, tlv.Blob]{Val: []byte("USD")},
	)

	payerKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	ir := testInvoiceRequest(t, offer, payerKey, 5000)

	err = ValidateInvoiceRequestForOffer(ir, &offers.Offer{}, 0)
	require.ErrorIs(t, err, ErrCurrencyNotSupported)
}

// TestValidateInvoiceRequestForOffer_Overflow verifies that offer_amount times
// the quantity is refused when it overflows, rather than wrapping to a small
// expected amount.
func TestValidateInvoiceRequestForOffer_Overflow(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	offer := testOffer(t, key, math.MaxUint64/2+1)
	setQuantityMax(offer, 0)

	payerKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	ir := testInvoiceRequest(t, offer, payerKey, 1)
	ir.InvreqQuantity = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType86, bolt12.TUint64]{Val: 2},
	)

	err = ValidateInvoiceRequestForOffer(ir, &offers.Offer{}, 0)
	require.ErrorIs(t, err, errAmountOverflow)
}
