package bolt12handler

import (
	"bytes"
	"math"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// TestPayOfferAmount verifies that the payer always pins invreq_amount: to the
// caller's amount, or to the offer's own amount times the quantity.
func TestPayOfferAmount(t *testing.T) {
	t.Parallel()

	key := testKey(t)

	fixed := testOffer(t, key, 1000)
	open := testOffer(t, key, 0)
	currency := testOffer(t, key, 5)
	currency.OfferCurrency = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType6, tlv.Blob]{Val: []byte("USD")},
	)
	huge := testOffer(t, key, math.MaxUint64/2+1)

	tests := []struct {
		name     string
		offer    *bolt12.Offer
		amount   uint64
		quantity uint64
		want     uint64
		wantErr  error
	}{
		{
			name:  "fixed offer, no amount",
			offer: fixed,
			want:  1000,
		},
		{
			name:     "fixed offer times quantity",
			offer:    fixed,
			quantity: 3,
			want:     3000,
		},
		{
			name:   "explicit amount wins",
			offer:  fixed,
			amount: 1500,
			want:   1500,
		},
		{
			name:    "open offer needs an amount",
			offer:   open,
			wantErr: ErrPayOfferAmountRequired,
		},
		{
			name:   "open offer with amount",
			offer:  open,
			amount: 42,
			want:   42,
		},
		{
			name:    "currency offer needs an amount",
			offer:   currency,
			wantErr: ErrCurrencyNeedsAmount,
		},
		{
			name:   "currency offer with amount",
			offer:  currency,
			amount: 7000,
			want:   7000,
		},
		{
			name:     "overflow",
			offer:    huge,
			quantity: 2,
			wantErr:  errAmountOverflow,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := PayOfferAmount(
				tc.offer, tc.amount, tc.quantity,
			)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestDerivePayerKey verifies that the payer key derivation is deterministic
// per key and offer, differs otherwise, and can be recovered from the
// metadata alone.
func TestDerivePayerKey(t *testing.T) {
	t.Parallel()

	secret := PayerSecret(testKey(t))
	offerA := [32]byte{0xaa}
	offerB := [32]byte{0xbb}
	key := []byte("idempotency-key")

	first, err := DerivePayerKey(secret, key, offerA)
	require.NoError(t, err)

	again, err := DerivePayerKey(secret, key, offerA)
	require.NoError(t, err)
	require.Equal(t, first.Metadata, again.Metadata)
	require.Equal(t, first.PrivKey.Serialize(), again.PrivKey.Serialize())

	otherKey, err := DerivePayerKey(secret, []byte("other"), offerA)
	require.NoError(t, err)
	require.NotEqual(t, first.Metadata, otherKey.Metadata)

	// The same key for another offer gives another payer id, so two
	// offers cannot link the payments.
	otherOffer, err := DerivePayerKey(secret, key, offerB)
	require.NoError(t, err)
	require.False(t, first.PrivKey.PubKey().IsEqual(
		otherOffer.PrivKey.PubKey(),
	))

	// Another node derives other keys from the same idempotency key.
	otherNode, err := DerivePayerKey(
		PayerSecret(newTestPrivKey(t)), key, offerA,
	)
	require.NoError(t, err)
	require.NotEqual(t, first.Metadata, otherNode.Metadata)

	// The payer key comes back from the metadata an invoice mirrors.
	recovered, err := PayerPrivKeyFromMetadata(secret, first.Metadata)
	require.NoError(t, err)
	require.Equal(t, first.PrivKey.Serialize(), recovered.Serialize())
}

// newTestPrivKey returns a fresh random private key.
func newTestPrivKey(t *testing.T) *btcec.PrivateKey {
	t.Helper()

	key, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	return key
}

// TestPayOfferParamsHash verifies that each payment parameter changes the
// hash, so a known key with any other parameter is refused.
func TestPayOfferParamsHash(t *testing.T) {
	t.Parallel()

	offer := [32]byte{0x01}
	base := PayOfferParamsHash(offer, 1000, 1, "note")

	require.Equal(t, base, PayOfferParamsHash(offer, 1000, 1, "note"))
	require.NotEqual(
		t, base, PayOfferParamsHash([32]byte{0x02}, 1000, 1, "note"),
	)
	require.NotEqual(t, base, PayOfferParamsHash(offer, 1001, 1, "note"))
	require.NotEqual(t, base, PayOfferParamsHash(offer, 1000, 2, "note"))
	require.NotEqual(t, base, PayOfferParamsHash(offer, 1000, 1, "other"))
}

// TestBuildInvoiceRequestWithPayerKey verifies that a request built with a
// derived payer key is byte-identical across builds, which is what lets a
// retry send the same request.
func TestBuildInvoiceRequestWithPayerKey(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	offer := testOffer(t, key, 1000)
	offerHash, err := bolt12.OfferHash(offer)
	require.NoError(t, err)

	payer, err := DerivePayerKey(
		PayerSecret(key), []byte("key"), offerHash,
	)
	require.NoError(t, err)

	build := func() []byte {
		ir, _, err := BuildInvoiceRequest(
			offer, WithAmount(1000), WithPayerKey(payer),
		)
		require.NoError(t, err)

		b, err := ir.EncodeSigned()
		require.NoError(t, err)

		return b
	}

	first := build()
	require.True(t, bytes.Equal(first, build()))

	ir, err := bolt12.DecodeInvoiceRequest(first)
	require.NoError(t, err)

	ir.InvreqMetadata.WhenSome(
		func(r tlv.RecordT[tlv.TlvType0, tlv.Blob]) {
			require.Equal(t, payer.Metadata, []byte(r.Val))
		},
	)
	ir.InvreqPayerID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType88, *btcec.PublicKey]) {
			require.True(t, r.Val.IsEqual(payer.PrivKey.PubKey()))
		},
	)
}
