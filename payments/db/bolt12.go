package paymentsdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/sqldb"
)

var (
	// ErrBolt12NotSupported is returned when a BOLT 12 payment operation
	// reaches a payments store that cannot hold BOLT 12 data.
	ErrBolt12NotSupported = errors.New("BOLT 12 payments are not " +
		"supported by this payments store")

	// ErrBolt12PaymentNotFound is returned when no offer payment uses the
	// requested idempotency key.
	ErrBolt12PaymentNotFound = errors.New("no BOLT 12 payment for the " +
		"idempotency key")

	// ErrBolt12KeyParamsMismatch is returned when an idempotency key is
	// used again with other payment parameters. The node never answers a
	// different request with the result of an old one.
	ErrBolt12KeyParamsMismatch = errors.New("idempotency key was used " +
		"with other payment parameters")

	// ErrBolt12KeyInUse is returned when an idempotency key already has a
	// payment that has not failed.
	ErrBolt12KeyInUse = errors.New("idempotency key already has a " +
		"payment that has not failed")

	// ErrBolt12KeyConsumed is returned when the payment of an idempotency
	// key was deleted. The node can no longer show that the payment
	// failed, so the key stays used for good.
	ErrBolt12KeyConsumed = errors.New("idempotency key belongs to a " +
		"deleted payment")
)

// Bolt12PaymentInfo binds a payment to the idempotency key of an offer
// payment. The key names one intended payment, so at most one payment that
// has not failed can belong to it.
type Bolt12PaymentInfo struct {
	// IdempotencyKey is the caller's key for the intended payment.
	IdempotencyKey []byte

	// OfferHash is the hash of the offer the payment answers.
	OfferHash [32]byte

	// ParamsHash commits to the parameters that define the payment. A
	// known key with other parameters is refused.
	ParamsHash [32]byte
}

// Bolt12PaymentRecord is the stored state of an idempotency key.
type Bolt12PaymentRecord struct {
	// PaymentHash is the hash of the key's current payment. It is nil
	// when that payment was deleted.
	PaymentHash *lntypes.Hash

	// OfferHash is the hash of the offer the key pays.
	OfferHash [32]byte

	// ParamsHash commits to the parameters of the key's payment.
	ParamsHash [32]byte
}

// Matches reports whether the record was created for the given payment.
func (r *Bolt12PaymentRecord) Matches(info *Bolt12PaymentInfo) bool {
	return r.OfferHash == info.OfferHash &&
		r.ParamsHash == info.ParamsHash
}

// fetchBolt12Payment returns the stored state of an idempotency key.
func fetchBolt12Payment(ctx context.Context, db SQLQueries,
	key []byte) (*Bolt12PaymentRecord, error) {

	row, err := db.FetchBolt12Payment(ctx, key)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, ErrBolt12PaymentNotFound

	case err != nil:
		return nil, fmt.Errorf("unable to fetch BOLT 12 payment: %w",
			err)
	}

	record := &Bolt12PaymentRecord{}
	copy(record.OfferHash[:], row.OfferHash)
	copy(record.ParamsHash[:], row.ParamsHash)

	if row.PaymentID.Valid {
		hash, err := lntypes.MakeHash(row.PaymentIdentifier)
		if err != nil {
			return nil, fmt.Errorf("invalid payment hash for "+
				"BOLT 12 payment: %w", err)
		}
		record.PaymentHash = &hash
	}

	return record, nil
}

// checkBolt12Key decides whether a new payment may start under the given
// idempotency key. It must run in the same transaction as the payment insert,
// before a failed payment with the same hash is deleted. A key is free when it
// is unknown, or when its payment failed and the parameters are the same.
func checkBolt12Key(ctx context.Context, cfg *sqldb.QueryConfig,
	db SQLQueries, info *Bolt12PaymentInfo) error {

	record, err := fetchBolt12Payment(ctx, db, info.IdempotencyKey)
	switch {
	case errors.Is(err, ErrBolt12PaymentNotFound):
		return nil

	case err != nil:
		return err
	}

	if !record.Matches(info) {
		return ErrBolt12KeyParamsMismatch
	}

	if record.PaymentHash == nil {
		return ErrBolt12KeyConsumed
	}

	payment, err := db.FetchPayment(ctx, record.PaymentHash[:])
	if err != nil {
		return fmt.Errorf("unable to fetch payment of idempotency "+
			"key: %w", err)
	}

	status, err := computePaymentStatusFromDB(ctx, cfg, db, payment)
	if err != nil {
		return fmt.Errorf("unable to compute payment status: %w", err)
	}

	// The status is failed only when no HTLC is in flight and none has
	// settled, so no money can still move for the old payment.
	if status != StatusFailed {
		return ErrBolt12KeyInUse
	}

	return nil
}

// FetchBolt12Payment returns the stored state of an idempotency key, or
// ErrBolt12PaymentNotFound when no offer payment uses it.
func (s *SQLStore) FetchBolt12Payment(ctx context.Context,
	key []byte) (*Bolt12PaymentRecord, error) {

	var record *Bolt12PaymentRecord
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		var err error
		record, err = fetchBolt12Payment(ctx, db, key)

		return err
	}, func() {
		record = nil
	})
	if err != nil {
		return nil, err
	}

	return record, nil
}

// offerPaymentIDs returns the IDs of the payments that answer the given offer.
func offerPaymentIDs(ctx context.Context, db SQLQueries,
	offerHash []byte) (map[int64]struct{}, error) {

	rows, err := db.FetchBolt12PaymentIDsByOffer(ctx, offerHash)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch offer payments: %w",
			err)
	}

	ids := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		if row.Valid {
			ids[row.Int64] = struct{}{}
		}
	}

	return ids, nil
}
