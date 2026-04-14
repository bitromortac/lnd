package bolt12handler

import (
	"context"
	"fmt"

	"github.com/lightningnetwork/lnd/clock"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/lightningnetwork/lnd/sqldb/sqlc"
)

// SQLInvReqQueries is the interface that defines the set of operations
// that can be executed against the invoice request store SQL database.
type SQLInvReqQueries interface {
	InsertInvoiceRequest(ctx context.Context,
		arg sqlc.InsertInvoiceRequestParams) (int64, error)

	FetchInvoiceRequestsByOfferID(ctx context.Context,
		offerID []byte) ([]sqlc.InvoiceRequestStore, error)
}

// BatchedSQLInvReqQueries combines the invoice request queries
// interface with batched transaction execution.
type BatchedSQLInvReqQueries interface {
	SQLInvReqQueries

	sqldb.BatchedTx[SQLInvReqQueries]
}

// SQLInvReqStore is the SQL-backed implementation of InvReqStore.
type SQLInvReqStore struct {
	db    BatchedSQLInvReqQueries
	clock clock.Clock
}

// NewSQLInvReqStore creates a new SQL-backed invoice request store.
func NewSQLInvReqStore(db BatchedSQLInvReqQueries,
	clock clock.Clock) *SQLInvReqStore {

	return &SQLInvReqStore{
		db:    db,
		clock: clock,
	}
}

// Save persists a complete negotiation record.
func (s *SQLInvReqStore) Save(ctx context.Context, offerID,
	invreqBytes, invoiceBytes, payerKey []byte) error {

	return s.db.ExecTx(
		ctx, sqldb.WriteTxOpt(),
		func(q SQLInvReqQueries) error {
			_, err := q.InsertInvoiceRequest(
				ctx, sqlc.InsertInvoiceRequestParams{
					OfferID:      offerID,
					InvreqBytes:  invreqBytes,
					InvoiceBytes: invoiceBytes,
					PayerKey:     payerKey,
					CreatedAt:    s.clock.Now().UTC(),
				},
			)

			return err
		},
		sqldb.NoOpReset,
	)
}

// FetchByOfferID returns all negotiation records for the given offer.
func (s *SQLInvReqStore) FetchByOfferID(ctx context.Context,
	offerID []byte) ([]*InvReqRecord, error) {

	var result []*InvReqRecord

	err := s.db.ExecTx(
		ctx, sqldb.ReadTxOpt(),
		func(q SQLInvReqQueries) error {
			rows, err := q.FetchInvoiceRequestsByOfferID(
				ctx, offerID,
			)
			if err != nil {
				return err
			}

			result = make([]*InvReqRecord, 0, len(rows))
			for _, row := range rows {
				result = append(result, &InvReqRecord{
					ID:           row.ID,
					OfferID:      row.OfferID,
					InvReqBytes:  row.InvreqBytes,
					InvoiceBytes: row.InvoiceBytes,
					PayerKey:     row.PayerKey,
					CreatedAt:    row.CreatedAt,
				})
			}

			return nil
		},
		sqldb.NoOpReset,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch invreqs by offer: %w", err)
	}

	return result, nil
}
