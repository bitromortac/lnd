package offers

import (
	"context"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/stretchr/testify/require"
)

// issuerIdentity wraps a pubkey as a Left Either for CreateOfferParams.
func issuerIdentity(
	key *btcec.PublicKey) fn.Either[*btcec.PublicKey, []lnwire.BlindedPath] {

	return fn.NewLeft[*btcec.PublicKey, []lnwire.BlindedPath](key)
}

// testIssuerKey generates a deterministic private key for testing.
func testIssuerKey(t *testing.T) *btcec.PrivateKey {
	t.Helper()

	// Use a fixed seed for deterministic tests.
	var seed [32]byte
	for i := range seed {
		seed[i] = byte(i + 1)
	}

	privKey, _ := btcec.PrivKeyFromBytes(seed[:])

	return privKey
}

// TestCreateOfferValid verifies that a valid offer is created, encoded,
// persisted, and retrievable.
func TestCreateOfferValid(t *testing.T) {
	t.Parallel()

	store := newTestSQLStore(t)
	ctx := context.Background()
	privKey := testIssuerKey(t)

	result, err := CreateOffer(
		ctx, store, CreateOfferParams{
			Identity: issuerIdentity(privKey.PubKey()),
			Description: "coffee",
			AmountMsat:  10000,
		},
	)
	require.NoError(t, err)
	require.NotEmpty(t, result.Encoded)
	require.NotEqual(t, [32]byte{}, result.OfferID)
	require.Greater(t, result.ID, int64(0))

	// Verify the encoded string decodes correctly.
	decoded, err := bolt12.DecodeOfferString(
		result.Encoded, time.Now(),
		[32]byte(*chaincfg.MainNetParams.GenesisHash),
	)
	require.NoError(t, err)

	// Verify the issuer ID matches.
	issuerPub := decoded.OfferIssuerID.UnwrapOrFailV(t)
	require.True(t, privKey.PubKey().IsEqual(issuerPub))

	// Verify the offer is persisted.
	got, err := store.GetOfferByOfferID(ctx, result.OfferID)
	require.NoError(t, err)
	require.Equal(t, result.Encoded, got.Encoded)
	require.Equal(t, "coffee", got.Description)
	require.True(t, got.HasAmount)
	require.Equal(t, uint64(10000), got.AmountMsat)
}

// TestCreateOfferNoAmount verifies that an offer without a fixed amount works
// (description is optional in this case).
func TestCreateOfferNoAmount(t *testing.T) {
	t.Parallel()

	store := newTestSQLStore(t)
	ctx := context.Background()
	privKey := testIssuerKey(t)

	result, err := CreateOffer(
		ctx, store, CreateOfferParams{
			Identity: issuerIdentity(privKey.PubKey()),
			Description: "tips",
		},
	)
	require.NoError(t, err)
	require.NotEmpty(t, result.Encoded)

	got, err := store.GetOffer(ctx, result.ID)
	require.NoError(t, err)
	require.False(t, got.HasAmount)
}

// TestCreateOfferMissingDescription verifies that creating an offer with an
// amount but no description is rejected.
func TestCreateOfferMissingDescription(t *testing.T) {
	t.Parallel()

	store := newTestSQLStore(t)
	ctx := context.Background()
	privKey := testIssuerKey(t)

	_, err := CreateOffer(
		ctx, store, CreateOfferParams{
			Identity:   issuerIdentity(privKey.PubKey()),
			AmountMsat: 10000,
		},
	)
	require.ErrorIs(t, err, ErrMissingDescription)
}

// TestCreateOfferMissingIssuerKey verifies that a nil issuer key is rejected.
func TestCreateOfferMissingIssuerKey(t *testing.T) {
	t.Parallel()

	store := newTestSQLStore(t)
	ctx := context.Background()

	_, err := CreateOffer(
		ctx, store, CreateOfferParams{
			Description: "test",
		},
	)
	require.ErrorIs(t, err, ErrMissingIssuerKey)
}

// TestCreateOfferWithExpiry verifies that an offer with an expiry is created
// and persisted correctly.
func TestCreateOfferWithExpiry(t *testing.T) {
	t.Parallel()

	store := newTestSQLStore(t)
	ctx := context.Background()
	privKey := testIssuerKey(t)

	result, err := CreateOffer(
		ctx, store, CreateOfferParams{
			Identity:       issuerIdentity(privKey.PubKey()),
			Description:    "limited time",
			AmountMsat:     5000,
			AbsoluteExpiry: 1735689600,
		},
	)
	require.NoError(t, err)

	got, err := store.GetOffer(ctx, result.ID)
	require.NoError(t, err)
	require.True(t, got.HasExpiry)
	require.Equal(t, uint64(1735689600), got.AbsoluteExpiry)
}

// TestCreateOfferWithQuantity verifies that an offer with quantity support is
// created and persisted correctly.
func TestCreateOfferWithQuantity(t *testing.T) {
	t.Parallel()

	store := newTestSQLStore(t)
	ctx := context.Background()
	privKey := testIssuerKey(t)

	qty := uint64(10)
	result, err := CreateOffer(
		ctx, store, CreateOfferParams{
			Identity: issuerIdentity(privKey.PubKey()),
			Description: "stickers",
			AmountMsat:  1000,
			QuantityMax: &qty,
		},
	)
	require.NoError(t, err)

	got, err := store.GetOffer(ctx, result.ID)
	require.NoError(t, err)
	require.True(t, got.HasQuantityMax)
	require.Equal(t, uint64(10), got.QuantityMax)
}

// TestCreateOfferDuplicate verifies that creating the same offer twice fails
// due to the unique offer_id constraint.
func TestCreateOfferDuplicate(t *testing.T) {
	t.Parallel()

	store := newTestSQLStore(t)
	ctx := context.Background()
	privKey := testIssuerKey(t)

	params := CreateOfferParams{
		Identity: issuerIdentity(privKey.PubKey()),
		Description: "coffee",
		AmountMsat:  10000,
	}

	_, err := CreateOffer(ctx, store, params)
	require.NoError(t, err)

	// Same parameters produce the same offer_id, so should fail.
	_, err = CreateOffer(ctx, store, params)
	require.Error(t, err)
}
