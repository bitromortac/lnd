package bolt12handler

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/lightningnetwork/lnd/bolt12"
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

// testOffer creates a codec offer from the test issuer key with a
// description and, when amountMsat is non-zero, an amount.
func testOffer(t *testing.T, key *btcec.PrivateKey,
	amountMsat uint64) *bolt12.Offer {

	t.Helper()

	offer := &bolt12.Offer{}
	offer.OfferIssuerID = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType22, *btcec.PublicKey]{
			Val: key.PubKey(),
		},
	)
	offer.OfferDescription = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType10, tlv.Blob]{
			Val: []byte("test offer"),
		},
	)

	if amountMsat > 0 {
		offer.OfferAmount = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType8, bolt12.TUint64]{
				Val: bolt12.TUint64(amountMsat),
			},
		)
	}

	return offer
}

// setQuantityMax sets offer_quantity_max on the offer.
func setQuantityMax(offer *bolt12.Offer, quantityMax uint64) {
	offer.OfferQuantityMax = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType20, bolt12.TUint64]{
			Val: bolt12.TUint64(quantityMax),
		},
	)
}

// testInvoiceRequest creates a minimal invoice request that mirrors the given
// offer's fields.
func testInvoiceRequest(t *testing.T, offer *bolt12.Offer,
	payerKey *btcec.PrivateKey,
	invreqAmount uint64) *bolt12.InvoiceRequest {

	t.Helper()

	ir, err := bolt12.NewInvoiceRequestFromOffer(
		offer, payerKey.PubKey(), []byte("test-metadata"),
		testChainHash(),
	)
	require.NoError(t, err)

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
