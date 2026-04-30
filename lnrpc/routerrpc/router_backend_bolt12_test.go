package routerrpc

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// TestInvoiceOfferHash verifies that a payment shows an offer hash only when
// its invoice answers an offer, and not for an invoice that answers an
// invoice request without an offer, which still carries offer-range fields.
func TestInvoiceOfferHash(t *testing.T) {
	t.Parallel()

	key, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	description := tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType10](tlv.Blob("refund")),
	)

	// An invoice for a request without an offer: offer_description, but
	// no offer_issuer_id and no offer_paths.
	offerless := &bolt12.Invoice{OfferDescription: description}
	require.Nil(t, invoiceOfferHash(offerless))

	// The same invoice for an offer with an issuer id.
	withOffer := &bolt12.Invoice{
		OfferDescription: description,
		OfferIssuerID: tlv.SomeRecordT(
			tlv.NewPrimitiveRecord[tlv.TlvType22](key.PubKey()),
		),
	}
	hash := invoiceOfferHash(withOffer)
	require.Len(t, hash, 32)

	want, err := bolt12.OfferHash(withOffer)
	require.NoError(t, err)
	require.Equal(t, want[:], hash)
}
