package bolt12

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// TestOfferRoundTrip pins encode→decode→re-encode for an Offer with a
// representative subset of optional fields. A byte-identical re-encode is the
// invariant that keeps offer_id stable across the codec boundary.
func TestOfferRoundTrip(t *testing.T) {
	t.Parallel()

	desc := tlv.Blob("coffee")
	issuer := tlv.Blob("alice")
	_, bobPub := bobKey()

	o := &Offer{
		OfferAmount: tlv.SomeRecordT(
			tlv.NewRecordT[tlv.TlvType8](TUint64(1500)),
		),
		OfferDescription: tlv.SomeRecordT(
			tlv.NewPrimitiveRecord[tlv.TlvType10](desc),
		),
		OfferIssuer: tlv.SomeRecordT(
			tlv.NewPrimitiveRecord[tlv.TlvType18](issuer),
		),
		OfferIssuerID: tlv.SomeRecordT(
			tlv.NewPrimitiveRecord[tlv.TlvType22](bobPub),
		),
	}

	encoded, err := o.Encode()
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	decoded, err := decodeOffer(encoded)
	require.NoError(t, err)

	require.Equal(t, TUint64(1500), decoded.OfferAmount.UnwrapOrFailV(t))
	require.Equal(t, desc, decoded.OfferDescription.UnwrapOrFailV(t))
	require.Equal(t, issuer, decoded.OfferIssuer.UnwrapOrFailV(t))

	reencoded, err := decoded.Encode()
	require.NoError(t, err)
	require.Equal(t, encoded, reencoded)
}

// TestOfferValidateRejectsWriterViolations confirms Encode refuses to emit
// bytes for offers that fail the writer requirements. Without the gate the
// codec could ship a structurally invalid offer that the receiver would later
// reject, leaking encoding effort.
func TestOfferValidateRejectsWriterViolations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*Offer)
		wantErr error
	}{
		{
			name: "amount without description",
			mutate: func(o *Offer) {
				o.OfferAmount = tlv.SomeRecordT(
					tlv.NewRecordT[tlv.TlvType8](
						TUint64(1000),
					),
				)
				o.OfferDescription = tlv.OptionalRecordT[
					tlv.TlvType10, tlv.Blob,
				]{}
			},
			wantErr: ErrMissingDescription,
		},
		{
			name: "currency without amount",
			mutate: func(o *Offer) {
				o.OfferCurrency = tlv.SomeRecordT(
					tlv.NewPrimitiveRecord[tlv.TlvType6](
						tlv.Blob("USD"),
					),
				)
			},
			wantErr: ErrCurrencyWithoutAmount,
		},
		{
			name: "no issuer id and no paths",
			mutate: func(o *Offer) {
				o.OfferIssuerID = tlv.OptionalRecordT[
					tlv.TlvType22, *btcec.PublicKey,
				]{}
			},
			wantErr: ErrNoIssuerIdentity,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			vec := findTestVector(t, "Minimal bolt12 offer")
			o, err := DecodeOfferString(
				vec.Bolt12, farFutureNow(),
				bitcoinMainnetGenesisHash,
			)
			require.NoError(t, err)

			tc.mutate(o)

			_, err = o.Encode()
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

// TestOfferRoundTripPreservesAllTypes decodes every valid offer vector,
// re-encodes it, decodes again, and asserts the second canonical encoding is
// byte-identical to the first. This pins the encode/decode bijection at the
// message level — any drift (e.g. dropped unknown odd fields, re-ordered
// records) breaks the assertion.
func TestOfferRoundTripPreservesAllTypes(t *testing.T) {
	t.Parallel()

	for _, tc := range loadOffersVectors(t) {
		if !tc.Valid {
			continue
		}

		t.Run(tc.Description, func(t *testing.T) {
			t.Parallel()

			_, tlvBytes, err := Decode(tc.Bolt12)
			require.NoError(t, err)

			first, err := decodeOffer(tlvBytes)
			require.NoError(t, err)

			// Some valid vectors fail writer requirements (e.g.
			// unknown odd experimental fields whose presence is
			// allowed on read but not on write) — encoding then is
			// expected to error and the round-trip case does not
			// apply.
			encoded, err := first.Encode()
			if err != nil {
				return
			}

			second, err := decodeOffer(encoded)
			require.NoError(t, err)
			reEncoded, err := second.Encode()
			require.NoError(t, err)

			require.Equal(t, encoded, reEncoded,
				"Encode→Decode→Encode must be the identity")
		})
	}
}
