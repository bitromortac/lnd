package invoices_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/clock"
	invpkg "github.com/lightningnetwork/lnd/invoices"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/stretchr/testify/require"
)

// makeBolt12TestDB creates a SQLite-backed invoice DB for testing.
func makeBolt12TestDB(t *testing.T) invpkg.InvoiceDB {
	t.Helper()

	db := sqldb.NewTestSqliteDB(t).BaseDB

	executor := sqldb.NewTransactionExecutor(
		db,
		func(tx *sql.Tx) invpkg.SQLInvoiceQueries {
			return db.WithTx(tx)
		},
	)

	testClock := clock.NewTestClock(time.Unix(1, 0))

	return invpkg.NewSQLStore(executor, testClock)
}

// TestBolt12InvoiceRoundTrip verifies that a BOLT 12 invoice with all new
// fields round-trips through insert and retrieval.
func TestBolt12InvoiceRoundTrip(t *testing.T) {
	t.Parallel()

	db := makeBolt12TestDB(t)
	ctx := context.Background()

	var (
		preimage lntypes.Preimage
		payAddr  [32]byte
	)
	_, err := rand.Read(preimage[:])
	require.NoError(t, err)
	_, err = rand.Read(payAddr[:])
	require.NoError(t, err)

	payHash := preimage.Hash()

	// Create test BOLT 12 identity fields.
	var invoiceNodeID [33]byte
	invoiceNodeID[0] = 0x02
	for i := 1; i < 33; i++ {
		invoiceNodeID[i] = byte(i)
	}

	var invreqPayerID [33]byte
	invreqPayerID[0] = 0x03
	for i := 1; i < 33; i++ {
		invreqPayerID[i] = byte(i + 50)
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
		IsBolt12:      true,
		InvoiceNodeID: invoiceNodeID[:],
		InvreqPayerID: invreqPayerID[:],
	}

	// Insert the invoice.
	_, err = db.AddInvoice(ctx, invoice, payHash)
	require.NoError(t, err)

	// Retrieve by hash.
	ref := invpkg.InvoiceRefByHash(payHash)
	got, err := db.LookupInvoice(ctx, ref)
	require.NoError(t, err)

	// Verify BOLT 12 fields.
	require.True(t, got.IsBolt12)
	require.Nil(t, got.OfferID)
	require.Equal(t, invoiceNodeID[:], got.InvoiceNodeID)
	require.Equal(t, invreqPayerID[:], got.InvreqPayerID)
}

// TestBolt11InvoiceBackwardCompat verifies that a BOLT 11 invoice (without BOLT
// 12 fields) still works correctly after the migration.
func TestBolt11InvoiceBackwardCompat(t *testing.T) {
	t.Parallel()

	db := makeBolt12TestDB(t)
	ctx := context.Background()

	// Create a standard BOLT 11 invoice (no BOLT 12 fields).
	invoice, err := randInvoice(lnwire.MilliSatoshi(5000))
	require.NoError(t, err)

	payHash := invoice.Terms.PaymentPreimage.Hash()

	_, err = db.AddInvoice(ctx, invoice, payHash)
	require.NoError(t, err)

	ref := invpkg.InvoiceRefByHash(payHash)
	got, err := db.LookupInvoice(ctx, ref)
	require.NoError(t, err)

	// Verify BOLT 12 fields are at their zero/nil defaults.
	require.False(t, got.IsBolt12)
	require.Nil(t, got.OfferID)
	require.Nil(t, got.InvoiceNodeID)
	require.Nil(t, got.InvreqPayerID)
}

// TestBolt12InvoiceNoOffer verifies that a BOLT 12 invoice without an offer FK
// (e.g., spontaneous invoice request) works correctly.
func TestBolt12InvoiceNoOffer(t *testing.T) {
	t.Parallel()

	db := makeBolt12TestDB(t)
	ctx := context.Background()

	var (
		preimage lntypes.Preimage
		payAddr  [32]byte
	)
	_, err := rand.Read(preimage[:])
	require.NoError(t, err)
	_, err = rand.Read(payAddr[:])
	require.NoError(t, err)

	payHash := preimage.Hash()

	var invoiceNodeID [33]byte
	invoiceNodeID[0] = 0x02

	invoice := &invpkg.Invoice{
		CreationDate: time.Unix(1, 0),
		Terms: invpkg.ContractTerm{
			Expiry:          7200 * time.Second,
			PaymentPreimage: &preimage,
			PaymentAddr:     payAddr,
			Value:           lnwire.MilliSatoshi(10000),
			Features:        emptyFeatures,
		},
		IsBolt12:      true,
		InvoiceNodeID: invoiceNodeID[:],
	}

	_, err = db.AddInvoice(ctx, invoice, payHash)
	require.NoError(t, err)

	ref := invpkg.InvoiceRefByHash(payHash)
	got, err := db.LookupInvoice(ctx, ref)
	require.NoError(t, err)

	require.True(t, got.IsBolt12)
	require.Nil(t, got.OfferID)
	require.Equal(t, invoiceNodeID[:], got.InvoiceNodeID)
	require.Nil(t, got.InvreqPayerID)
}
