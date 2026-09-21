package bolt12handler

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/offers"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// TestValidateInvoiceRequestForOffer_HappyPath verifies that a valid invoice
// request passes validation.
func TestValidateInvoiceRequestForOffer_HappyPath(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	offer := testOffer(t, key, 10000)

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	ir := testInvoiceRequest(t, offer, payerKey, 10000)

	now := uint64(time.Now().Unix())
	err := ValidateInvoiceRequestForOffer(ir, &offers.Offer{}, now)
	require.NoError(t, err)
}

// TestValidateInvoiceRequestForOffer_DisabledOffer verifies rejection of
// requests for disabled offers.
func TestValidateInvoiceRequestForOffer_DisabledOffer(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	offer := testOffer(t, key, 10000)

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	ir := testInvoiceRequest(t, offer, payerKey, 10000)

	err := ValidateInvoiceRequestForOffer(
		ir, &offers.Offer{IsDisabled: true},
		uint64(
			time.Now().Unix(),
		),
	)
	require.ErrorIs(t, err, ErrOfferDisabled)
}

// TestValidateInvoiceRequestForOffer_ExpiredOffer verifies rejection of
// requests for expired offers.
func TestValidateInvoiceRequestForOffer_ExpiredOffer(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	offer := testOffer(t, key, 10000)
	setAbsoluteExpiry(offer, 1000) // Long past.

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	ir := testInvoiceRequest(t, offer, payerKey, 10000)

	err := ValidateInvoiceRequestForOffer(
		ir, &offers.Offer{},
		uint64(
			time.Now().Unix(),
		),
	)
	require.ErrorIs(t, err, ErrOfferExpired)
}

// TestValidateInvoiceRequestForOffer_ExpirySecond verifies that a request
// that arrives at the offer_absolute_expiry second is rejected, and that a
// request one second earlier passes.
func TestValidateInvoiceRequestForOffer_ExpirySecond(t *testing.T) {
	t.Parallel()

	const expiry = 1735689600

	key := testKey(t)
	offer := testOffer(t, key, 10000)
	setAbsoluteExpiry(offer, expiry)

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	ir := testInvoiceRequest(t, offer, payerKey, 10000)

	err := ValidateInvoiceRequestForOffer(ir, &offers.Offer{}, expiry-1)
	require.NoError(t, err)

	err = ValidateInvoiceRequestForOffer(ir, &offers.Offer{}, expiry)
	require.ErrorIs(t, err, ErrOfferExpired)
}

// setAbsoluteExpiry sets offer_absolute_expiry on the offer.
func setAbsoluteExpiry(offer *bolt12.Offer, expiry uint64) {
	offer.OfferAbsoluteExpiry = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType14, bolt12.TUint64]{
			Val: bolt12.TUint64(expiry),
		},
	)
}

// TestValidateInvoiceRequestForOffer_AmountBelowExpected verifies rejection
// when invreq_amount is below the expected amount.
func TestValidateInvoiceRequestForOffer_AmountBelowExpected(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	offer := testOffer(t, key, 10000)

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	// Set invreq_amount below the offer amount.
	ir := testInvoiceRequest(t, offer, payerKey, 5000)

	err := ValidateInvoiceRequestForOffer(
		ir, &offers.Offer{},
		uint64(
			time.Now().Unix(),
		),
	)
	require.ErrorIs(t, err, ErrAmountBelowExpected)
}

// TestValidateInvoiceRequestForOffer_NoAmountNoInvreq verifies that when the
// offer has no amount, invreq_amount must be present.
func TestValidateInvoiceRequestForOffer_NoAmountNoInvreq(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	offer := testOffer(t, key, 0) // No fixed amount.

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	// No invreq_amount either.
	ir := testInvoiceRequest(t, offer, payerKey, 0)

	err := ValidateInvoiceRequestForOffer(
		ir, &offers.Offer{},
		uint64(
			time.Now().Unix(),
		),
	)
	require.ErrorIs(t, err, ErrMissingInvreqAmount)
}

// TestValidateInvoiceRequestForOffer_QuantityNotExpected verifies rejection
// when invreq_quantity is present but offer has no quantity_max.
func TestValidateInvoiceRequestForOffer_QuantityNotExpected(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	offer := testOffer(t, key, 10000)

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	ir := testInvoiceRequest(t, offer, payerKey, 10000)

	// Add invreq_quantity when offer has no quantity_max.
	qty := bolt12.TUint64(5)
	ir.InvreqQuantity = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType86, bolt12.TUint64]{
			Val: qty,
		},
	)

	err := ValidateInvoiceRequestForOffer(
		ir, &offers.Offer{},
		uint64(
			time.Now().Unix(),
		),
	)
	require.ErrorIs(t, err, ErrQuantityNotExpected)
}

// TestValidateInvoiceRequestForOffer_MissingQuantity verifies rejection when
// offer has quantity_max but invreq_quantity is absent.
func TestValidateInvoiceRequestForOffer_MissingQuantity(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	offer := testOffer(t, key, 10000)
	setQuantityMax(offer, 10)

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	// No invreq_quantity.
	ir := testInvoiceRequest(t, offer, payerKey, 10000)

	err := ValidateInvoiceRequestForOffer(
		ir, &offers.Offer{},
		uint64(
			time.Now().Unix(),
		),
	)
	require.ErrorIs(t, err, ErrMissingQuantity)
}

// TestValidateInvoiceRequestForOffer_QuantityWithAmount verifies correct amount
// computation when quantity is involved.
func TestValidateInvoiceRequestForOffer_QuantityWithAmount(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	offer := testOffer(t, key, 1000) // 1000 msat per item.
	setQuantityMax(offer, 10)

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	// 5 items at 1000 msat = 5000 msat expected.
	ir := testInvoiceRequest(t, offer, payerKey, 5000)
	qty := bolt12.TUint64(5)
	ir.InvreqQuantity = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType86, bolt12.TUint64]{
			Val: qty,
		},
	)

	err := ValidateInvoiceRequestForOffer(
		ir, &offers.Offer{},
		uint64(
			time.Now().Unix(),
		),
	)
	require.NoError(t, err)

	// Now try with insufficient amount for 5 items.
	ir2 := testInvoiceRequest(t, offer, payerKey, 4000)
	ir2.InvreqQuantity = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType86, bolt12.TUint64]{
			Val: qty,
		},
	)

	err = ValidateInvoiceRequestForOffer(
		ir2, &offers.Offer{},
		uint64(
			time.Now().Unix(),
		),
	)
	require.ErrorIs(t, err, ErrAmountBelowExpected)
}
