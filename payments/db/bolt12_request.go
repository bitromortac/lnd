package paymentsdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/lightningnetwork/lnd/sqldb/sqlc"
)

var (
	// ErrBolt12InvoiceRequestNotFound is returned when no published
	// invoice request matches the lookup.
	ErrBolt12InvoiceRequestNotFound = errors.New("BOLT 12 invoice " +
		"request not found")

	// ErrBolt12InvoiceRequestPaid is returned when a published invoice
	// request already has a payment that has not failed. A request pays
	// at most once.
	ErrBolt12InvoiceRequestPaid = errors.New("BOLT 12 invoice request " +
		"already has a payment that has not failed")

	// ErrBolt12InvoiceRequestConsumed is returned when the payment of a
	// used invoice request was deleted. The node can no longer show that
	// the payment failed, so the request stays used for good.
	ErrBolt12InvoiceRequestConsumed = errors.New("BOLT 12 invoice " +
		"request belongs to a deleted payment")
)

// Bolt12InvoiceRequest is an invoice request without an offer that this node
// published as a payer. Such a request is an offer to send money: the payee
// answers it with an invoice, and the node pays that invoice at most once.
type Bolt12InvoiceRequest struct {
	// ID is the database ID of the request.
	ID int64

	// IdempotencyKey is the caller's key. The metadata and the payer key
	// of the request are derived from it.
	IdempotencyKey []byte

	// Metadata is the invreq_metadata of the request. An invoice mirrors
	// it, so it finds the request an incoming invoice answers.
	Metadata []byte

	// Encoded is the signed request as an lnr1 string.
	Encoded string

	// Amount is the invreq_amount the request pays.
	Amount lnwire.MilliSatoshi

	// ExpiresAt is the time after which the node no longer pays an
	// invoice for the request. The zero time means no expiry.
	ExpiresAt time.Time

	// ExpectedNodeID is the invoice_node_id agreed with the payee out of
	// band, or nil when the user approves each invoice.
	ExpectedNodeID []byte

	// FeeLimit is the routing fee budget the operator approved for paying
	// an invoice from the expected node at once. Zero selects the default
	// fee limit.
	FeeLimit lnwire.MilliSatoshi

	// Used reports whether a payment ever started for the request.
	Used bool

	// PaymentHash is the hash of the request's current payment. It is nil
	// when no payment exists, also when the payment was deleted.
	PaymentHash *lntypes.Hash

	// CreatedAt is the time the request was created.
	CreatedAt time.Time
}

// Expired reports whether the request no longer pays at the given time.
func (r *Bolt12InvoiceRequest) Expired(now time.Time) bool {
	return !r.ExpiresAt.IsZero() && now.After(r.ExpiresAt)
}

// Bolt12RequestBinding binds a payment to a published invoice request. The
// payments store checks and binds the request in the transaction that
// creates the payment.
type Bolt12RequestBinding struct {
	// RequestID is the database ID of the published request.
	RequestID int64
}

// Bolt12InvoiceRequestStore persists the invoice requests without an offer
// that this node publishes as a payer. Only the SQL store implements it,
// because the table is a development migration of native SQL.
type Bolt12InvoiceRequestStore interface {
	// InsertBolt12InvoiceRequest stores a published request and returns
	// its database ID.
	InsertBolt12InvoiceRequest(ctx context.Context,
		req *Bolt12InvoiceRequest) (int64, error)

	// FetchBolt12InvoiceRequestByKey returns the request created with the
	// given idempotency key.
	FetchBolt12InvoiceRequestByKey(ctx context.Context,
		key []byte) (*Bolt12InvoiceRequest, error)

	// FetchBolt12InvoiceRequestByMetadata returns the request with the
	// given invreq_metadata.
	FetchBolt12InvoiceRequestByMetadata(ctx context.Context,
		metadata []byte) (*Bolt12InvoiceRequest, error)

	// ListBolt12InvoiceRequests returns all published requests.
	ListBolt12InvoiceRequests(
		ctx context.Context) ([]*Bolt12InvoiceRequest, error)
}

// A compile-time check that the SQL store holds published requests.
var _ Bolt12InvoiceRequestStore = (*SQLStore)(nil)

// unmarshalBolt12InvoiceRequest converts a database row into a request.
func unmarshalBolt12InvoiceRequest(
	row sqlc.FetchBolt12InvoiceRequestByIDRow) (*Bolt12InvoiceRequest,
	error) {

	req := &Bolt12InvoiceRequest{
		ID:             row.ID,
		IdempotencyKey: row.IdempotencyKey,
		Metadata:       row.InvreqMetadata,
		Encoded:        row.Encoded,
		Amount:         lnwire.MilliSatoshi(row.AmountMsat),
		ExpectedNodeID: row.ExpectedNodeID,
		FeeLimit:       lnwire.MilliSatoshi(row.FeeLimitMsat),
		Used:           row.Used,
		CreatedAt:      row.CreatedAt,
	}
	if row.ExpiresAt.Valid {
		req.ExpiresAt = row.ExpiresAt.Time
	}

	if row.PaymentID.Valid {
		hash, err := lntypes.MakeHash(row.PaymentIdentifier)
		if err != nil {
			return nil, fmt.Errorf("invalid payment hash for "+
				"invoice request: %w", err)
		}
		req.PaymentHash = &hash
	}

	return req, nil
}

// InsertBolt12InvoiceRequest stores a published request and returns its
// database ID.
func (s *SQLStore) InsertBolt12InvoiceRequest(ctx context.Context,
	req *Bolt12InvoiceRequest) (int64, error) {

	params := sqlc.InsertBolt12InvoiceRequestParams{
		IdempotencyKey: req.IdempotencyKey,
		InvreqMetadata: req.Metadata,
		Encoded:        req.Encoded,
		AmountMsat:     int64(req.Amount),
		ExpectedNodeID: req.ExpectedNodeID,
		FeeLimitMsat:   int64(req.FeeLimit),
		CreatedAt:      req.CreatedAt.UTC(),
	}
	if !req.ExpiresAt.IsZero() {
		params.ExpiresAt = sqldb.SQLTime(req.ExpiresAt.UTC())
	}

	var id int64
	err := s.db.ExecTx(ctx, sqldb.WriteTxOpt(), func(db SQLQueries) error {
		var err error
		id, err = db.InsertBolt12InvoiceRequest(ctx, params)

		return err
	}, sqldb.NoOpReset)
	if err != nil {
		return 0, fmt.Errorf("unable to insert invoice request: %w",
			err)
	}

	return id, nil
}

// fetchBolt12InvoiceRequest runs one lookup query and converts its row.
func (s *SQLStore) fetchBolt12InvoiceRequest(ctx context.Context,
	fetch func(db SQLQueries) (sqlc.FetchBolt12InvoiceRequestByIDRow,
		error)) (*Bolt12InvoiceRequest, error) {

	var req *Bolt12InvoiceRequest
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		row, err := fetch(db)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return ErrBolt12InvoiceRequestNotFound

		case err != nil:
			return fmt.Errorf("unable to fetch invoice request: "+
				"%w", err)
		}

		req, err = unmarshalBolt12InvoiceRequest(row)

		return err
	}, func() {
		req = nil
	})
	if err != nil {
		return nil, err
	}

	return req, nil
}

// FetchBolt12InvoiceRequestByKey returns the request created with the given
// idempotency key.
func (s *SQLStore) FetchBolt12InvoiceRequestByKey(ctx context.Context,
	key []byte) (*Bolt12InvoiceRequest, error) {

	return s.fetchBolt12InvoiceRequest(ctx, func(db SQLQueries) (
		sqlc.FetchBolt12InvoiceRequestByIDRow, error) {

		row, err := db.FetchBolt12InvoiceRequestByKey(ctx, key)

		return sqlc.FetchBolt12InvoiceRequestByIDRow(row), err
	})
}

// FetchBolt12InvoiceRequestByMetadata returns the request with the given
// invreq_metadata.
func (s *SQLStore) FetchBolt12InvoiceRequestByMetadata(ctx context.Context,
	metadata []byte) (*Bolt12InvoiceRequest, error) {

	return s.fetchBolt12InvoiceRequest(ctx, func(db SQLQueries) (
		sqlc.FetchBolt12InvoiceRequestByIDRow, error) {

		row, err := db.FetchBolt12InvoiceRequestByMetadata(
			ctx, metadata,
		)

		return sqlc.FetchBolt12InvoiceRequestByIDRow(row), err
	})
}

// ListBolt12InvoiceRequests returns all published requests.
func (s *SQLStore) ListBolt12InvoiceRequests(
	ctx context.Context) ([]*Bolt12InvoiceRequest, error) {

	var reqs []*Bolt12InvoiceRequest
	err := s.db.ExecTx(ctx, sqldb.ReadTxOpt(), func(db SQLQueries) error {
		rows, err := db.ListBolt12InvoiceRequests(ctx)
		if err != nil {
			return fmt.Errorf("unable to list invoice requests: %w",
				err)
		}

		for _, row := range rows {
			req, err := unmarshalBolt12InvoiceRequest(
				sqlc.FetchBolt12InvoiceRequestByIDRow(row),
			)
			if err != nil {
				return err
			}
			reqs = append(reqs, req)
		}

		return nil
	}, func() {
		reqs = nil
	})
	if err != nil {
		return nil, err
	}

	return reqs, nil
}

// checkBolt12Request decides whether a new payment may start for a published
// invoice request. It must run in the same transaction as the payment insert,
// before a failed payment with the same hash is deleted. A request is free
// when no payment ever started for it, or when its payment failed. This is
// the idempotency key invariant with the request as the key.
func checkBolt12Request(ctx context.Context, cfg *sqldb.QueryConfig,
	db SQLQueries, binding *Bolt12RequestBinding) error {

	row, err := db.FetchBolt12InvoiceRequestByID(ctx, binding.RequestID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ErrBolt12InvoiceRequestNotFound

	case err != nil:
		return fmt.Errorf("unable to fetch invoice request: %w", err)
	}

	if !row.Used {
		return nil
	}

	if !row.PaymentID.Valid {
		return ErrBolt12InvoiceRequestConsumed
	}

	payment, err := db.FetchPayment(ctx, row.PaymentIdentifier)
	if err != nil {
		return fmt.Errorf("unable to fetch payment of invoice "+
			"request: %w", err)
	}

	status, err := computePaymentStatusFromDB(ctx, cfg, db, payment)
	if err != nil {
		return fmt.Errorf("unable to compute payment status: %w", err)
	}

	// The status is failed only when no HTLC is in flight and none has
	// settled, so no money can still move for the old payment.
	if status != StatusFailed {
		return ErrBolt12InvoiceRequestPaid
	}

	return nil
}
