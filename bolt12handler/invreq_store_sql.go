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

	FetchInvoiceRequestsByOfferHash(ctx context.Context,
		offerHash []byte) ([]sqlc.InvoiceRequestStore, error)
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
func (s *SQLInvReqStore) Save(ctx context.Context, offerHash,
	invreqBytes, invoiceBytes, payerKey []byte) error {

	return s.db.ExecTx(
		ctx, sqldb.WriteTxOpt(),
		func(q SQLInvReqQueries) error {
			_, err := q.InsertInvoiceRequest(
				ctx, sqlc.InsertInvoiceRequestParams{
					OfferHash:    offerHash,
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

// FetchByOfferHash returns all negotiation records for the given offer.
func (s *SQLInvReqStore) FetchByOfferHash(ctx context.Context,
	offerHash []byte) ([]*InvReqRecord, error) {

	var result []*InvReqRecord

	err := s.db.ExecTx(
		ctx, sqldb.ReadTxOpt(),
		func(q SQLInvReqQueries) error {
			rows, err := q.FetchInvoiceRequestsByOfferHash(
				ctx, offerHash,
			)
			if err != nil {
				return err
			}

			result = make([]*InvReqRecord, 0, len(rows))
			for _, row := range rows {
				result = append(result, &InvReqRecord{
					ID:           row.ID,
					OfferHash:    row.OfferHash,
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
