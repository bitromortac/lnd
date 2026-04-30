package bolt12handler

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/offers"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// testChainHash returns the Bitcoin mainnet genesis hash, the chain
// the codec defaults to when offer_chains/invreq_chain is absent.
func testChainHash() [32]byte {
	return *chaincfg.MainNetParams.GenesisHash
}

// testKey returns a deterministic private key for testing.
func testKey(t *testing.T) *btcec.PrivateKey {
	t.Helper()

	var seed [32]byte
	for i := range seed {
		seed[i] = byte(i + 1)
	}

	privKey, _ := btcec.PrivKeyFromBytes(seed[:])

	return privKey
}

// testOffer creates a test offer with the given parameters.
func testOffer(t *testing.T, key *btcec.PrivateKey,
	amountMsat uint64) *offers.Offer {

	t.Helper()

	var issuerNodeID [33]byte
	copy(
		issuerNodeID[:],
		key.PubKey().SerializeCompressed(),
	)

	offer := &offers.Offer{
		ID:           1,
		IssuerNodeID: issuerNodeID,
		Description:  "test offer",
		HasAmount:    amountMsat > 0,
		AmountMsat:   amountMsat,
		CreatedAt:    time.Now().UTC(),
	}

	return offer
}

// testInvoiceRequest creates a minimal invoice request that mirrors the given
// offer's fields.
func testInvoiceRequest(t *testing.T, offer *offers.Offer,
	payerKey *btcec.PrivateKey,
	invreqAmount uint64) *bolt12.InvoiceRequest {

	t.Helper()

	ir := &bolt12.InvoiceRequest{}

	// Mirror offer fields.
	issuerPub, err := btcec.ParsePubKey(offer.IssuerNodeID[:])
	require.NoError(t, err)
	ir.OfferIssuerID = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType22, *btcec.PublicKey]{
			Val: issuerPub,
		},
	)
	ir.OfferDescription = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType10, tlv.Blob]{
			Val: []byte(offer.Description),
		},
	)

	if offer.HasAmount {
		amt := bolt12.TUint64(offer.AmountMsat)
		ir.OfferAmount = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType8, bolt12.TUint64]{
				Val: amt,
			},
		)
	}

	if offer.HasQuantityMax {
		qty := bolt12.TUint64(offer.QuantityMax)
		ir.OfferQuantityMax = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType20, bolt12.TUint64]{
				Val: qty,
			},
		)
	}

	// Set payer fields.
	ir.InvreqPayerID = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType88](payerKey.PubKey()),
	)
	ir.InvreqMetadata = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType0, tlv.Blob]{
			Val: []byte("test-metadata"),
		},
	)

	if invreqAmount > 0 {
		amt := bolt12.TUint64(invreqAmount)
		ir.InvreqAmount = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType82, bolt12.TUint64]{
				Val: amt,
			},
		)
	}

	return ir
}

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
	err := ValidateInvoiceRequestForOffer(ir, offer, now)
	require.NoError(t, err)
}

// TestValidateInvoiceRequestForOffer_DisabledOffer verifies rejection of
// requests for disabled offers.
func TestValidateInvoiceRequestForOffer_DisabledOffer(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	offer := testOffer(t, key, 10000)
	offer.IsDisabled = true

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	ir := testInvoiceRequest(t, offer, payerKey, 10000)

	err := ValidateInvoiceRequestForOffer(
		ir, offer,
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
	offer.HasExpiry = true
	offer.AbsoluteExpiry = 1000 // Long past.

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	ir := testInvoiceRequest(t, offer, payerKey, 10000)

	err := ValidateInvoiceRequestForOffer(
		ir, offer,
		uint64(
			time.Now().Unix(),
		),
	)
	require.ErrorIs(t, err, ErrOfferExpired)
}

// TestValidateInvoiceRequestForOffer_IssuerIDMismatch verifies rejection when
// the offer_issuer_id does not match.
func TestValidateInvoiceRequestForOffer_IssuerIDMismatch(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	offer := testOffer(t, key, 10000)

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	ir := testInvoiceRequest(t, offer, payerKey, 10000)

	// Tamper with the issuer ID in the request by using a different key.
	ir.OfferIssuerID = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType22, *btcec.PublicKey]{
			Val: payerKey.PubKey(),
		},
	)

	err := ValidateInvoiceRequestForOffer(
		ir, offer,
		uint64(
			time.Now().Unix(),
		),
	)
	require.ErrorIs(t, err, ErrOfferFieldMismatch)
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
		ir, offer,
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
		ir, offer,
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
		ir, offer,
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
	offer.HasQuantityMax = true
	offer.QuantityMax = 10

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	// No invreq_quantity.
	ir := testInvoiceRequest(t, offer, payerKey, 10000)

	err := ValidateInvoiceRequestForOffer(
		ir, offer,
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
	offer.HasQuantityMax = true
	offer.QuantityMax = 10

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
		ir, offer,
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
		ir2, offer,
		uint64(
			time.Now().Unix(),
		),
	)
	require.ErrorIs(t, err, ErrAmountBelowExpected)
}
