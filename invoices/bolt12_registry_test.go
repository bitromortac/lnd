//go:build test_db_sqlite

package invoices_test

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/lightningnetwork/lnd/clock"
	invpkg "github.com/lightningnetwork/lnd/invoices"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/record"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/lightningnetwork/lnd/sqldb/sqlc"
	"github.com/stretchr/testify/require"
)

// errTestOfferDisabled stands in for the handler's disabled-offer error.
var errTestOfferDisabled = errors.New("offer is disabled")

// fakeReconstructor rebuilds a fixed BOLT 12 invoice, as the handler's
// reconstructor does from a valid envelope, and reports whether its offer is
// disabled.
type fakeReconstructor struct {
	invoice  *invpkg.Invoice
	disabled atomic.Bool
}

// ReconstructInvoice returns a copy of the fixed invoice, or refuses it when
// the offer is disabled.
func (f *fakeReconstructor) ReconstructInvoice(context.Context, []byte,
	chainhash.Hash, lntypes.Hash) (*invpkg.Invoice, error) {

	if f.disabled.Load() {
		return nil, errTestOfferDisabled
	}

	invoice := *f.invoice

	return &invoice, nil
}

// CheckOfferActive refuses the offer once it is disabled.
func (f *fakeReconstructor) CheckOfferActive(context.Context, []byte) error {
	if f.disabled.Load() {
		return errTestOfferDisabled
	}

	return nil
}

// newBolt12RegistryContext creates a registry over a BOLT 12 enabled SQLite
// store with one offer, and a reconstructor for an invoice of that offer.
func newBolt12RegistryContext(t *testing.T) (*testContext,
	*fakeReconstructor, lntypes.Preimage, chainhash.Hash) {

	t.Helper()

	db := sqldb.NewTestSqliteDB(t).BaseDB
	testClock := clock.NewTestClock(testTime)

	offerHash := [32]byte{0xaa}
	offerID, err := db.Queries.InsertOffer(
		t.Context(), sqlc.InsertOfferParams{
			Hash:      offerHash[:],
			Encoded:   "lno1test",
			CreatedAt: testTime,
		},
	)
	require.NoError(t, err)

	preimage := lntypes.Preimage{0x01, 0x02}
	pathID := chainhash.Hash{0x03}
	payerID := make([]byte, 33)
	payerID[0] = 0x02

	reconstructor := &fakeReconstructor{
		invoice: &invpkg.Invoice{
			CreationDate: testTime,
			Terms: invpkg.ContractTerm{
				Expiry:          time.Hour,
				PaymentPreimage: &preimage,
				PaymentAddr:     pathID,
				Value:           testInvoiceAmount,
				Features:        lnwire.EmptyFeatureVector(),
			},
			IsBolt12:      true,
			OfferID:       &offerID,
			OfferHash:     offerHash[:],
			InvreqPayerID: payerID,
		},
	}

	cfg := defaultRegistryConfig()
	cfg.Bolt12Reconstructor = reconstructor

	makeDB := func(t *testing.T) (invpkg.InvoiceDB, *clock.TestClock) {
		executor := sqldb.NewTransactionExecutor(
			db, func(tx *sql.Tx) invpkg.SQLInvoiceQueries {
				return db.WithTx(tx)
			},
		)

		store := invpkg.NewSQLStore(
			executor, testClock,
		)

		return store, testClock
	}

	return newTestContext(t, &cfg, makeDB), reconstructor, preimage,
		pathID
}

// notifyBolt12Shard sends one MPP shard of half the invoice amount to the
// registry.
func notifyBolt12Shard(t *testing.T, ctx *testContext,
	preimage lntypes.Preimage, pathID chainhash.Hash,
	htlcID uint64) (invpkg.HtlcResolution, chan interface{}) {

	t.Helper()

	payload := &mockPayload{
		mpp:          record.NewMPP(testInvoiceAmount, pathID),
		pathID:       &pathID,
		envelope:     []byte{0x01},
		totalAmtMsat: testInvoiceAmount,
	}
	hodlChan := make(chan interface{}, 1)

	resolution, err := ctx.registry.NotifyExitHopHtlc(
		preimage.Hash(), testInvoiceAmount/2, testHtlcExpiry,
		testCurrentHeight, getCircuitKey(htlcID), hodlChan, nil,
		payload,
	)
	require.NoError(t, err)

	return resolution, hodlChan
}

// TestBolt12SettlesWhileOfferActive verifies that the reconstructed invoice
// settles when both shards of an MPP payment arrive while the offer is
// active.
func TestBolt12SettlesWhileOfferActive(t *testing.T) {
	t.Parallel()

	ctx, _, preimage, pathID := newBolt12RegistryContext(t)

	resolution, _ := notifyBolt12Shard(t, ctx, preimage, pathID, 1)
	require.Nil(t, resolution)

	resolution, _ = notifyBolt12Shard(t, ctx, preimage, pathID, 2)
	checkSettleResolution(t, resolution, preimage)
}

// TestBolt12RefusesShardAfterOfferDisabled verifies that a later shard is
// refused when the offer is disabled after the first shard arrived. No shard
// settles, so the payment of the disabled offer never completes.
func TestBolt12RefusesShardAfterOfferDisabled(t *testing.T) {
	t.Parallel()

	ctx, reconstructor, preimage, pathID := newBolt12RegistryContext(t)

	resolution, hodlChan := notifyBolt12Shard(t, ctx, preimage, pathID, 1)
	require.Nil(t, resolution)

	reconstructor.disabled.Store(true)

	resolution, _ = notifyBolt12Shard(t, ctx, preimage, pathID, 2)
	checkFailResolution(t, resolution, invpkg.ResultInvoiceNotOpen)

	// The first shard is still held, not settled.
	select {
	case <-hodlChan:
		t.Fatal("first shard was resolved")

	default:
	}

	invoice, err := ctx.registry.LookupInvoice(
		t.Context(), preimage.Hash(),
	)
	require.NoError(t, err)
	require.Equal(t, invpkg.ContractOpen, invoice.State)
}

// TestBolt12RefusesFirstShardOfDisabledOffer verifies that no invoice is
// created when the first shard arrives for a disabled offer.
func TestBolt12RefusesFirstShardOfDisabledOffer(t *testing.T) {
	t.Parallel()

	ctx, reconstructor, preimage, pathID := newBolt12RegistryContext(t)
	reconstructor.disabled.Store(true)

	resolution, _ := notifyBolt12Shard(t, ctx, preimage, pathID, 1)
	checkFailResolution(t, resolution, invpkg.ResultInvoiceNotFound)

	_, err := ctx.registry.LookupInvoice(t.Context(), preimage.Hash())
	require.ErrorIs(t, err, invpkg.ErrInvoiceNotFound)
}

// TestBolt12SettlesInvoiceWithoutOffer verifies that the disabled-offer check
// skips an invoice that answers an invoice request without an offer. Such an
// invoice is stored when it is created, so it never goes through the
// reconstructor, and it has no offer to disable.
func TestBolt12SettlesInvoiceWithoutOffer(t *testing.T) {
	t.Parallel()

	ctx, reconstructor, preimage, pathID := newBolt12RegistryContext(t)

	// The reconstructor reports every offer as disabled, so a settled
	// payment proves that the check did not run.
	reconstructor.disabled.Store(true)

	invoice := *reconstructor.invoice
	invoice.OfferID = nil
	invoice.OfferHash = nil
	_, err := ctx.registry.AddInvoice(t.Context(), &invoice, preimage.Hash())
	require.NoError(t, err)

	resolution, _ := notifyBolt12Shard(t, ctx, preimage, pathID, 1)
	require.Nil(t, resolution)

	resolution, _ = notifyBolt12Shard(t, ctx, preimage, pathID, 2)
	checkSettleResolution(t, resolution, preimage)
}
