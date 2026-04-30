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

// TestDerivePayer verifies that the payer derivation is deterministic per
// key and offer, differs otherwise, and can be recovered from the metadata
// alone.
func TestDerivePayer(t *testing.T) {
	t.Parallel()

	signer := NewPrivKeySigner(testKey(t))
	offerA := [32]byte{0xaa}
	offerB := [32]byte{0xbb}
	key := []byte("idempotency-key")

	first, err := DerivePayer(signer, key, offerA)
	require.NoError(t, err)
	firstID, err := first.PubKey()
	require.NoError(t, err)

	again, err := DerivePayer(signer, key, offerA)
	require.NoError(t, err)
	require.Equal(t, first.Metadata, again.Metadata)
	require.Equal(t, first.Key, again.Key)

	otherKey, err := DerivePayer(signer, []byte("other"), offerA)
	require.NoError(t, err)
	require.NotEqual(t, first.Metadata, otherKey.Metadata)

	// The same key for another offer gives another payer id, so two
	// offers cannot link the payments.
	otherOffer, err := DerivePayer(signer, key, offerB)
	require.NoError(t, err)
	otherOfferID, err := otherOffer.PubKey()
	require.NoError(t, err)
	require.False(t, firstID.IsEqual(otherOfferID))

	// The payer id is not the node key.
	require.False(t, firstID.IsEqual(signer.NodePubKey()))

	// Another node derives other keys from the same idempotency key.
	otherNode, err := DerivePayer(
		NewPrivKeySigner(newTestPrivKey(t)), key, offerA,
	)
	require.NoError(t, err)
	require.NotEqual(t, first.Metadata, otherNode.Metadata)

	// The payer key comes back from the metadata an invoice mirrors.
	recovered, err := PayerFromMetadata(signer, first.Metadata)
	require.NoError(t, err)
	require.Equal(t, first.Key, recovered.Key)
}

// TestDerivePayerKeyRing verifies that the key ring signer derives the same
// payer as a signer with the raw node key, so local and remote signing give
// the same payer ids.
func TestDerivePayerKeyRing(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	key := []byte("idempotency-key")
	offerHash := [32]byte{0xaa}

	local, err := DerivePayer(NewPrivKeySigner(nodeKey), key, offerHash)
	require.NoError(t, err)

	keyRing, err := DerivePayer(testKeyRingSigner(nodeKey), key, offerHash)
	require.NoError(t, err)
	require.Equal(t, local.Metadata, keyRing.Metadata)
	require.Equal(t, local.Key, keyRing.Key)

	localID, err := local.PubKey()
	require.NoError(t, err)
	keyRingID, err := keyRing.PubKey()
	require.NoError(t, err)
	require.True(t, localID.IsEqual(keyRingID))
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

// TestBuildInvoiceRequestWithDerivedPayer verifies that a request built with
// a derived payer is byte-identical across builds, which is what lets a retry
// send the same request.
func TestBuildInvoiceRequestWithDerivedPayer(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	offer := testOffer(t, key, 1000)
	offerHash, err := bolt12.OfferHash(offer)
	require.NoError(t, err)

	payer, err := DerivePayer(
		NewPrivKeySigner(key), []byte("key"), offerHash,
	)
	require.NoError(t, err)
	payerID, err := payer.PubKey()
	require.NoError(t, err)

	build := func() []byte {
		ir, err := BuildInvoiceRequest(offer, payer, WithAmount(1000))
		require.NoError(t, err)

		b, err := ir.EncodeSigned()
		require.NoError(t, err)

		return b
	}

	first := build()
	require.True(t, bytes.Equal(first, build()))

	// The reader checks the signature against invreq_payer_id.
	ir, err := bolt12.DecodeInvoiceRequest(first)
	require.NoError(t, err)

	ir.InvreqMetadata.WhenSome(
		func(r tlv.RecordT[tlv.TlvType0, tlv.Blob]) {
			require.Equal(t, payer.Metadata, []byte(r.Val))
		},
	)
	ir.InvreqPayerID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType88, *btcec.PublicKey]) {
			require.True(t, r.Val.IsEqual(payerID))
		},
	)
}
