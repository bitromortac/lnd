//go:build test_db_sqlite || test_db_postgres

package paymentsdb

import (
	"bytes"
	"testing"

	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/stretchr/testify/require"
)

// testBolt12Info returns the BOLT 12 binding of a payment with the given key,
// offer and parameters.
func testBolt12Info(key, offer, params byte) *Bolt12PaymentInfo {
	info := &Bolt12PaymentInfo{
		IdempotencyKey: bytes.Repeat([]byte{key}, 32),
	}
	info.OfferHash[0] = offer
	info.ParamsHash[0] = params

	return info
}

// initBolt12Payment starts a payment with a new hash under the given BOLT 12
// binding and returns its hash and preimage.
func initBolt12Payment(t *testing.T, db DB, info *Bolt12PaymentInfo) (
	lntypes.Hash, lntypes.Preimage, error) {

	t.Helper()

	creationInfo, preimage := genInfo(t)
	creationInfo.Bolt12 = info

	hash := creationInfo.PaymentIdentifier
	err := db.InitPayment(t.Context(), hash, creationInfo)

	return hash, preimage, err
}

// settleBolt12Payment settles the payment with one HTLC. Attempt IDs are
// unique across all payments, so each payment of a test needs its own.
func settleBolt12Payment(t *testing.T, db DB, hash lntypes.Hash,
	preimage lntypes.Preimage, attemptID uint64) {

	t.Helper()

	attempt := genAttemptWithHash(t, attemptID, genSessionKey(t), hash)
	_, err := db.RegisterAttempt(t.Context(), hash, attempt)
	require.NoError(t, err)

	_, err = db.SettleAttempt(
		t.Context(), hash, attemptID,
		&HTLCSettleInfo{Preimage: preimage},
	)
	require.NoError(t, err)
}

// failBolt12Payment fails the payment before any HTLC was sent.
func failBolt12Payment(t *testing.T, db DB, hash lntypes.Hash) {
	t.Helper()

	_, err := db.Fail(t.Context(), hash, FailureReasonNoRoute)
	require.NoError(t, err)
}

// requireKeyPayment asserts that the key points to the payment with the
// given hash.
func requireKeyPayment(t *testing.T, db DB, info *Bolt12PaymentInfo,
	hash lntypes.Hash) {

	t.Helper()

	record, err := db.FetchBolt12Payment(t.Context(), info.IdempotencyKey)
	require.NoError(t, err)
	require.True(t, record.Matches(info))
	require.NotNil(t, record.PaymentHash)
	require.Equal(t, hash, *record.PaymentHash)
}

// TestBolt12KeyBindsPayment verifies that a new key is stored with its payment
// and that an unknown key is reported as not found.
func TestBolt12KeyBindsPayment(t *testing.T) {
	t.Parallel()

	db, _ := NewTestDB(t)
	info := testBolt12Info(1, 1, 1)

	_, err := db.FetchBolt12Payment(t.Context(), info.IdempotencyKey)
	require.ErrorIs(t, err, ErrBolt12PaymentNotFound)

	hash, _, err := initBolt12Payment(t, db, info)
	require.NoError(t, err)

	requireKeyPayment(t, db, info, hash)
}

// TestBolt12KeyRefusesSecondPayment verifies that a key with a payment that is
// in flight or succeeded refuses a second payment, also for a new invoice with
// a new hash.
func TestBolt12KeyRefusesSecondPayment(t *testing.T) {
	t.Parallel()

	db, _ := NewTestDB(t)
	info := testBolt12Info(1, 1, 1)

	hash, preimage, err := initBolt12Payment(t, db, info)
	require.NoError(t, err)

	_, _, err = initBolt12Payment(t, db, info)
	require.ErrorIs(t, err, ErrBolt12KeyInUse)

	settleBolt12Payment(t, db, hash, preimage, 0)

	_, _, err = initBolt12Payment(t, db, info)
	require.ErrorIs(t, err, ErrBolt12KeyInUse)

	requireKeyPayment(t, db, info, hash)
}

// TestBolt12KeyParamsMismatch verifies that a known key with other parameters
// or for another offer is refused.
func TestBolt12KeyParamsMismatch(t *testing.T) {
	t.Parallel()

	db, _ := NewTestDB(t)
	info := testBolt12Info(1, 1, 1)

	hash, _, err := initBolt12Payment(t, db, info)
	require.NoError(t, err)
	failBolt12Payment(t, db, hash)

	// The first payment failed, so only the parameters stop these.
	_, _, err = initBolt12Payment(t, db, testBolt12Info(1, 1, 2))
	require.ErrorIs(t, err, ErrBolt12KeyParamsMismatch)

	_, _, err = initBolt12Payment(t, db, testBolt12Info(1, 2, 1))
	require.ErrorIs(t, err, ErrBolt12KeyParamsMismatch)
}

// TestBolt12KeyRetryAfterFailure verifies that a failed payment frees its key,
// both for a new invoice and for the same invoice again, and that the key then
// points to the retry.
func TestBolt12KeyRetryAfterFailure(t *testing.T) {
	t.Parallel()

	db, _ := NewTestDB(t)
	info := testBolt12Info(1, 1, 1)

	firstHash, _, err := initBolt12Payment(t, db, info)
	require.NoError(t, err)
	failBolt12Payment(t, db, firstHash)

	// The payee answered the retry with a new invoice.
	secondHash, _, err := initBolt12Payment(t, db, info)
	require.NoError(t, err)
	requireKeyPayment(t, db, info, secondHash)

	// The new payment is in flight again, so the key is in use.
	_, _, err = initBolt12Payment(t, db, info)
	require.ErrorIs(t, err, ErrBolt12KeyInUse)

	// The payee caches by metadata and answers with the same invoice.
	// The store replaces the failed payment of that hash, and the key
	// follows it.
	failBolt12Payment(t, db, secondHash)

	creationInfo, _ := genInfo(t)
	creationInfo.PaymentIdentifier = secondHash
	creationInfo.Bolt12 = info
	err = db.InitPayment(t.Context(), secondHash, creationInfo)
	require.NoError(t, err)
	requireKeyPayment(t, db, info, secondHash)
}

// TestBolt12KeyDeletedPayment verifies that a key whose payment was deleted
// stays used for good.
func TestBolt12KeyDeletedPayment(t *testing.T) {
	t.Parallel()

	db, _ := NewTestDB(t)
	info := testBolt12Info(1, 1, 1)

	hash, preimage, err := initBolt12Payment(t, db, info)
	require.NoError(t, err)
	settleBolt12Payment(t, db, hash, preimage, 0)

	require.NoError(t, db.DeletePayment(t.Context(), hash, false))

	record, err := db.FetchBolt12Payment(t.Context(), info.IdempotencyKey)
	require.NoError(t, err)
	require.Nil(t, record.PaymentHash)

	_, _, err = initBolt12Payment(t, db, info)
	require.ErrorIs(t, err, ErrBolt12KeyConsumed)
}
