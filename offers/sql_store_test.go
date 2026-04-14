package offers

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/clock"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/stretchr/testify/require"
)

// newTestSQLStore creates a SQLite-backed offer store for testing.
func newTestSQLStore(t *testing.T) *SQLStore {
	t.Helper()

	db := sqldb.NewTestSqliteDB(t).BaseDB

	executor := sqldb.NewTransactionExecutor(
		db,
		func(tx *sql.Tx) SQLOfferQueries {
			return db.WithTx(tx)
		},
	)

	testClock := clock.NewTestClock(time.Now())

	return NewSQLStore(executor, testClock)
}

// testOffer returns a populated Offer suitable for testing.
func testOffer(t *testing.T) *Offer {
	t.Helper()

	var issuerNodeID [33]byte
	issuerNodeID[0] = 0x02
	for i := 1; i < 33; i++ {
		issuerNodeID[i] = byte(i)
	}

	encoded := "lno1qgsqvgnwgcg35z6ee2h3yczraddm72xrfua" +
		"9uve2rlrm9deu7xyfzrcgqyqs"
	offerID := sha256.Sum256([]byte(encoded))

	return &Offer{
		OfferID:        offerID,
		Encoded:        encoded,
		IssuerNodeID:   issuerNodeID,
		Description:    "test offer",
		AmountMsat:     10000,
		HasAmount:      true,
		AbsoluteExpiry: 1735689600,
		HasExpiry:      true,
		QuantityMax:    10,
		HasQuantityMax: true,
		CreatedAt:      time.Now().UTC().Truncate(time.Second),
	}
}

// TestInsertAndGetOffer verifies that an offer round-trips through insert and
// retrieval by both database ID and offer ID.
func TestInsertAndGetOffer(t *testing.T) {
	t.Parallel()

	store := newTestSQLStore(t)
	ctx := context.Background()
	offer := testOffer(t)

	// Insert the offer.
	id, err := store.InsertOffer(ctx, offer)
	require.NoError(t, err)
	require.Greater(t, id, int64(0))

	// Retrieve by database ID.
	got, err := store.GetOffer(ctx, id)
	require.NoError(t, err)
	require.Equal(t, offer.OfferID, got.OfferID)
	require.Equal(t, offer.Encoded, got.Encoded)
	require.Equal(t, offer.IssuerNodeID, got.IssuerNodeID)
	require.Equal(t, offer.Description, got.Description)
	require.Equal(t, offer.AmountMsat, got.AmountMsat)
	require.True(t, got.HasAmount)
	require.Equal(t, offer.AbsoluteExpiry, got.AbsoluteExpiry)
	require.True(t, got.HasExpiry)
	require.Equal(t, offer.QuantityMax, got.QuantityMax)
	require.True(t, got.HasQuantityMax)
	require.False(t, got.IsDisabled)

	// Retrieve by offer ID hash.
	got2, err := store.GetOfferByOfferID(ctx, offer.OfferID)
	require.NoError(t, err)
	require.Equal(t, got.ID, got2.ID)
	require.Equal(t, got.Encoded, got2.Encoded)
}

// TestInsertDuplicateOffer verifies that inserting an offer with the same
// offer_id fails.
func TestInsertDuplicateOffer(t *testing.T) {
	t.Parallel()

	store := newTestSQLStore(t)
	ctx := context.Background()
	offer := testOffer(t)

	_, err := store.InsertOffer(ctx, offer)
	require.NoError(t, err)

	_, err = store.InsertOffer(ctx, offer)
	require.Error(t, err)
}

// TestListOffers verifies listing offers with and without the active-only
// filter.
func TestListOffers(t *testing.T) {
	t.Parallel()

	store := newTestSQLStore(t)
	ctx := context.Background()

	// Insert two offers.
	offer1 := testOffer(t)
	id1, err := store.InsertOffer(ctx, offer1)
	require.NoError(t, err)

	offer2 := testOffer(t)
	offer2.OfferID = sha256.Sum256([]byte("different"))
	offer2.Encoded = "lno1different"
	_, err = store.InsertOffer(ctx, offer2)
	require.NoError(t, err)

	// List all — should return 2.
	all, err := store.ListOffers(ctx, false)
	require.NoError(t, err)
	require.Len(t, all, 2)

	// Disable the first offer.
	err = store.DisableOffer(ctx, id1)
	require.NoError(t, err)

	// List all — still 2.
	all, err = store.ListOffers(ctx, false)
	require.NoError(t, err)
	require.Len(t, all, 2)

	// List active only — should return 1.
	active, err := store.ListOffers(ctx, true)
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, offer2.Encoded, active[0].Encoded)
}

// TestDisableOffer verifies that disabling an offer marks it as disabled and
// that disabling a nonexistent offer returns an error.
func TestDisableOffer(t *testing.T) {
	t.Parallel()

	store := newTestSQLStore(t)
	ctx := context.Background()
	offer := testOffer(t)

	id, err := store.InsertOffer(ctx, offer)
	require.NoError(t, err)

	// Disable it.
	err = store.DisableOffer(ctx, id)
	require.NoError(t, err)

	// Verify it is disabled.
	got, err := store.GetOffer(ctx, id)
	require.NoError(t, err)
	require.True(t, got.IsDisabled)

	// Disable nonexistent offer.
	err = store.DisableOffer(ctx, 99999)
	require.Error(t, err)
}

// TestInsertOfferNullableFields verifies that an offer with no optional fields
// round-trips correctly.
func TestInsertOfferNullableFields(t *testing.T) {
	t.Parallel()

	store := newTestSQLStore(t)
	ctx := context.Background()

	var issuerNodeID [33]byte
	issuerNodeID[0] = 0x03

	offerID := sha256.Sum256([]byte("minimal"))

	offer := &Offer{
		OfferID:      offerID,
		Encoded:      "lno1minimal",
		IssuerNodeID: issuerNodeID,
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
	}

	id, err := store.InsertOffer(ctx, offer)
	require.NoError(t, err)

	got, err := store.GetOffer(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "", got.Description)
	require.False(t, got.HasAmount)
	require.Equal(t, uint64(0), got.AmountMsat)
	require.False(t, got.HasExpiry)
	require.False(t, got.HasQuantityMax)
	require.Equal(t, "", got.Currency)
}
