//go:build test_db_sqlite || test_db_postgres

package paymentsdb

import (
	"bytes"
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/stretchr/testify/require"
)

// insertTestRequest stores a published invoice request and returns it with
// its database ID.
func insertTestRequest(t *testing.T, db DB, tag byte) *Bolt12InvoiceRequest {
	t.Helper()

	store, ok := db.(Bolt12InvoiceRequestStore)
	require.True(t, ok)

	req := &Bolt12InvoiceRequest{
		IdempotencyKey: bytes.Repeat([]byte{tag}, 32),
		Metadata:       bytes.Repeat([]byte{tag + 1}, 32),
		Encoded:        "lnr1test",
		Amount:         5000,
		FeeLimit:       100,
		ExpiresAt:      time.Unix(2000, 0).UTC(),
		CreatedAt:      time.Unix(1000, 0).UTC(),
	}

	id, err := store.InsertBolt12InvoiceRequest(t.Context(), req)
	require.NoError(t, err)
	req.ID = id

	return req
}

// initRequestPayment starts a payment with a new hash for the published
// request.
func initRequestPayment(t *testing.T, db DB, requestID int64) (lntypes.Hash,
	lntypes.Preimage, error) {

	t.Helper()

	creationInfo, preimage := genInfo(t)
	creationInfo.Bolt12Request = &Bolt12RequestBinding{
		RequestID: requestID,
	}

	hash := creationInfo.PaymentIdentifier
	err := db.InitPayment(t.Context(), hash, creationInfo)

	return hash, preimage, err
}

// TestBolt12InvoiceRequestRoundTrip verifies that a published request is
// found by its key, by its metadata and in the list.
func TestBolt12InvoiceRequestRoundTrip(t *testing.T) {
	t.Parallel()

	db, _ := NewTestDB(t)
	store := db.(Bolt12InvoiceRequestStore)
	req := insertTestRequest(t, db, 1)

	byKey, err := store.FetchBolt12InvoiceRequestByKey(
		t.Context(), req.IdempotencyKey,
	)
	require.NoError(t, err)
	require.Equal(t, req, byKey)

	byMetadata, err := store.FetchBolt12InvoiceRequestByMetadata(
		t.Context(), req.Metadata,
	)
	require.NoError(t, err)
	require.Equal(t, req, byMetadata)

	list, err := store.ListBolt12InvoiceRequests(t.Context())
	require.NoError(t, err)
	require.Equal(t, []*Bolt12InvoiceRequest{req}, list)

	_, err = store.FetchBolt12InvoiceRequestByMetadata(
		t.Context(), []byte("unknown"),
	)
	require.ErrorIs(t, err, ErrBolt12InvoiceRequestNotFound)

	require.True(t, req.Expired(time.Unix(2001, 0)))
	require.False(t, req.Expired(time.Unix(2000, 0)))
}

// TestBolt12InvoiceRequestPaysOnce verifies that a published request pays at
// most once: a second invoice is refused while the first payment is in flight
// and after it settled.
func TestBolt12InvoiceRequestPaysOnce(t *testing.T) {
	t.Parallel()

	db, _ := NewTestDB(t)
	store := db.(Bolt12InvoiceRequestStore)
	req := insertTestRequest(t, db, 1)

	hash, preimage, err := initRequestPayment(t, db, req.ID)
	require.NoError(t, err)

	_, _, err = initRequestPayment(t, db, req.ID)
	require.ErrorIs(t, err, ErrBolt12InvoiceRequestPaid)

	settleBolt12Payment(t, db, hash, preimage, 0)

	_, _, err = initRequestPayment(t, db, req.ID)
	require.ErrorIs(t, err, ErrBolt12InvoiceRequestPaid)

	got, err := store.FetchBolt12InvoiceRequestByKey(
		t.Context(), req.IdempotencyKey,
	)
	require.NoError(t, err)
	require.True(t, got.Used)
	require.Equal(t, &hash, got.PaymentHash)
}

// TestBolt12InvoiceRequestRetryAfterFailure verifies that a failed payment
// frees the request for a new invoice, and that the request then points to
// the new payment.
func TestBolt12InvoiceRequestRetryAfterFailure(t *testing.T) {
	t.Parallel()

	db, _ := NewTestDB(t)
	store := db.(Bolt12InvoiceRequestStore)
	req := insertTestRequest(t, db, 1)

	first, _, err := initRequestPayment(t, db, req.ID)
	require.NoError(t, err)
	failBolt12Payment(t, db, first)

	second, _, err := initRequestPayment(t, db, req.ID)
	require.NoError(t, err)

	got, err := store.FetchBolt12InvoiceRequestByKey(
		t.Context(), req.IdempotencyKey,
	)
	require.NoError(t, err)
	require.Equal(t, &second, got.PaymentHash)
}

// TestBolt12InvoiceRequestDeletedPayment verifies that a request whose payment
// was deleted stays used for good.
func TestBolt12InvoiceRequestDeletedPayment(t *testing.T) {
	t.Parallel()

	db, _ := NewTestDB(t)
	req := insertTestRequest(t, db, 1)

	hash, preimage, err := initRequestPayment(t, db, req.ID)
	require.NoError(t, err)
	settleBolt12Payment(t, db, hash, preimage, 0)

	require.NoError(t, db.DeletePayment(t.Context(), hash, false))

	_, _, err = initRequestPayment(t, db, req.ID)
	require.ErrorIs(t, err, ErrBolt12InvoiceRequestConsumed)
}
