//go:build test_db_sqlite

package invoices_test

import (
	"crypto/rand"
	"database/sql"
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/clock"
	invpkg "github.com/lightningnetwork/lnd/invoices"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/lightningnetwork/lnd/sqldb/sqlc"
	"github.com/stretchr/testify/require"
)

// bolt12TestDB bundles an invoice store with the raw queries the tests use to
// set up offers and to look into the BOLT 12 side table.
type bolt12TestDB struct {
	store   invpkg.InvoiceDB
	queries *sqlc.Queries
}

// makeBolt12TestDB creates a SQLite-backed invoice store. The SQLite test
// build applies the development migrations, so the BOLT 12 side table exists
// whatever the options are.
func makeBolt12TestDB(t *testing.T,
	opts ...invpkg.SQLStoreOption) *bolt12TestDB {

	t.Helper()

	db := sqldb.NewTestSqliteDB(t).BaseDB

	executor := sqldb.NewTransactionExecutor(
		db,
		func(tx *sql.Tx) invpkg.SQLInvoiceQueries {
			return db.WithTx(tx)
		},
	)

	testClock := clock.NewTestClock(time.Unix(1, 0))

	return &bolt12TestDB{
		store:   invpkg.NewSQLStore(executor, testClock, opts...),
		queries: db.Queries,
	}
}

// insertTestOffer stores an offer and returns its database ID and hash.
func insertTestOffer(t *testing.T, db *bolt12TestDB) (int64, [32]byte) {
	t.Helper()

	var hash [32]byte
	_, err := rand.Read(hash[:])
	require.NoError(t, err)

	id, err := db.queries.InsertOffer(t.Context(), sqlc.InsertOfferParams{
		Hash:      hash[:],
		Encoded:   "lno1test",
		CreatedAt: time.Unix(1, 0),
	})
	require.NoError(t, err)

	return id, hash
}

// newBolt12Invoice returns a BOLT 12 invoice for the offer with the given ID,
// as the reconstructor builds it at settlement. It has no payment request.
func newBolt12Invoice(t *testing.T, offerID *int64) (*invpkg.Invoice,
	lntypes.Hash) {

	t.Helper()

	var (
		preimage lntypes.Preimage
		payAddr  [32]byte
	)
	_, err := rand.Read(preimage[:])
	require.NoError(t, err)
	_, err = rand.Read(payAddr[:])
	require.NoError(t, err)

	var payerID [33]byte
	payerID[0] = 0x03
	for i := 1; i < 33; i++ {
		payerID[i] = byte(i + 50)
	}

	invoice := &invpkg.Invoice{
		CreationDate: time.Unix(1, 0),
		Terms: invpkg.ContractTerm{
			Expiry:          7200 * time.Second,
			PaymentPreimage: &preimage,
			PaymentAddr:     payAddr,
			Value:           lnwire.MilliSatoshi(10000),
			Features:        emptyFeatures,
		},
		IsBolt12:       true,
		OfferID:        offerID,
		InvreqPayerID:  payerID[:],
		InvreqQuantity: 3,
	}

	return invoice, preimage.Hash()
}

// TestBolt12InvoiceRoundTrip verifies that the BOLT 12 data of an invoice
// round-trips through the side table, and that the offer hash comes from the
// offer.
func TestBolt12InvoiceRoundTrip(t *testing.T) {
	t.Parallel()

	db := makeBolt12TestDB(t, invpkg.WithBolt12())
	ctx := t.Context()

	offerID, offerHash := insertTestOffer(t, db)
	invoice, payHash := newBolt12Invoice(t, &offerID)

	_, err := db.store.AddInvoice(ctx, invoice, payHash)
	require.NoError(t, err)

	got, err := db.store.LookupInvoice(ctx, invpkg.InvoiceRefByHash(payHash))
	require.NoError(t, err)

	require.True(t, got.IsBolt12)
	require.NotNil(t, got.OfferID)
	require.Equal(t, offerID, *got.OfferID)
	require.Equal(t, offerHash[:], got.OfferHash)
	require.Equal(t, invoice.InvreqPayerID, got.InvreqPayerID)
	require.Equal(t, uint64(3), got.InvreqQuantity)

	// A settled BOLT 12 invoice has no payment request, but it is not a
	// keysend invoice.
	require.Empty(t, got.PaymentRequest)
	require.False(t, got.IsKeysend())
}

// TestBolt11InvoiceWithBolt12Store verifies that a BOLT 11 invoice has no side
// table row and reads back without BOLT 12 data.
func TestBolt11InvoiceWithBolt12Store(t *testing.T) {
	t.Parallel()

	db := makeBolt12TestDB(t, invpkg.WithBolt12())
	ctx := t.Context()

	invoice, err := randInvoice(lnwire.MilliSatoshi(5000))
	require.NoError(t, err)

	payHash := invoice.Terms.PaymentPreimage.Hash()

	addIndex, err := db.store.AddInvoice(ctx, invoice, payHash)
	require.NoError(t, err)

	got, err := db.store.LookupInvoice(ctx, invpkg.InvoiceRefByHash(payHash))
	require.NoError(t, err)

	require.False(t, got.IsBolt12)
	require.Zero(t, got.InvreqQuantity)
	require.Nil(t, got.OfferID)
	require.Nil(t, got.OfferHash)
	require.Nil(t, got.InvreqPayerID)

	_, err = db.queries.FetchBolt12Invoice(ctx, int64(addIndex))
	require.ErrorIs(t, err, sql.ErrNoRows)
}

// TestBolt12InvoiceStoreDisabled verifies that a store without BOLT 12 refuses
// a BOLT 12 invoice and never reads the side table.
func TestBolt12InvoiceStoreDisabled(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	db := makeBolt12TestDB(t)
	offerID, _ := insertTestOffer(t, db)
	invoice, payHash := newBolt12Invoice(t, &offerID)

	_, err := db.store.AddInvoice(ctx, invoice, payHash)
	require.ErrorIs(t, err, invpkg.ErrBolt12Disabled)

	_, err = db.store.LookupInvoice(ctx, invpkg.InvoiceRefByHash(payHash))
	require.ErrorIs(t, err, invpkg.ErrInvoiceNotFound)
}

// TestBolt12InvoiceWithoutOffer verifies that an invoice that answers an
// invoice request without an offer round-trips with an empty offer link and
// is still marked as a BOLT 12 invoice.
func TestBolt12InvoiceWithoutOffer(t *testing.T) {
	t.Parallel()

	db := makeBolt12TestDB(t, invpkg.WithBolt12())
	ctx := t.Context()

	invoice, payHash := newBolt12Invoice(t, nil)

	_, err := db.store.AddInvoice(ctx, invoice, payHash)
	require.NoError(t, err)

	got, err := db.store.LookupInvoice(ctx, invpkg.InvoiceRefByHash(payHash))
	require.NoError(t, err)

	require.True(t, got.IsBolt12)
	require.Nil(t, got.OfferID)
	require.Nil(t, got.OfferHash)
	require.Equal(t, invoice.InvreqPayerID, got.InvreqPayerID)
	require.Equal(t, uint64(3), got.InvreqQuantity)
}

// TestBolt12InvoiceDeleteCascades verifies that deleting a BOLT 12 invoice also
// deletes its side table row, as the registry does for an open BOLT 12 invoice
// whose HTLCs all failed.
func TestBolt12InvoiceDeleteCascades(t *testing.T) {
	t.Parallel()

	db := makeBolt12TestDB(t, invpkg.WithBolt12())
	ctx := t.Context()

	offerID, _ := insertTestOffer(t, db)
	invoice, payHash := newBolt12Invoice(t, &offerID)

	addIndex, err := db.store.AddInvoice(ctx, invoice, payHash)
	require.NoError(t, err)

	_, err = db.queries.FetchBolt12Invoice(ctx, int64(addIndex))
	require.NoError(t, err)

	payAddr := invoice.Terms.PaymentAddr
	err = db.store.DeleteInvoice(ctx, []invpkg.InvoiceDeleteRef{{
		PayHash:  payHash,
		PayAddr:  &payAddr,
		AddIndex: addIndex,
	}})
	require.NoError(t, err)

	_, err = db.queries.FetchBolt12Invoice(ctx, int64(addIndex))
	require.ErrorIs(t, err, sql.ErrNoRows)
}
