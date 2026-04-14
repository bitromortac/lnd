package offers

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lightningnetwork/lnd/clock"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/lightningnetwork/lnd/sqldb/sqlc"
)

// SQLOfferQueries is the interface that defines the set of operations that can
// be executed against the offers SQL database.
type SQLOfferQueries interface {
	InsertOffer(ctx context.Context,
		arg sqlc.InsertOfferParams) (int64, error)

	GetOfferByID(ctx context.Context, id int64) (sqlc.Offer, error)

	GetOfferByOfferID(ctx context.Context,
		offerID []byte) (sqlc.Offer, error)

	ListOffers(ctx context.Context,
		arg sqlc.ListOffersParams) ([]sqlc.Offer, error)

	DisableOffer(ctx context.Context, id int64) (sql.Result, error)
}

// BatchedSQLOfferQueries combines the offer queries interface with batched
// transaction execution.
type BatchedSQLOfferQueries interface {
	SQLOfferQueries

	sqldb.BatchedTx[SQLOfferQueries]
}

// SQLStore is the SQL-backed implementation of the Store interface.
type SQLStore struct {
	db    BatchedSQLOfferQueries
	clock clock.Clock
}

// NewSQLStore creates a new SQL-backed offer store.
func NewSQLStore(db BatchedSQLOfferQueries, clock clock.Clock) *SQLStore {

	return &SQLStore{
		db:    db,
		clock: clock,
	}
}

// InsertOffer persists a new offer and returns its database ID.
func (s *SQLStore) InsertOffer(ctx context.Context, offer *Offer) (int64,
	error) {

	var id int64

	err := s.db.ExecTx(
		ctx, sqldb.WriteTxOpt(),
		func(q SQLOfferQueries) error {
			params := sqlc.InsertOfferParams{
				OfferID:      offer.OfferID[:],
				Encoded:      offer.Encoded,
				IssuerNodeID: offer.IssuerNodeID[:],
				CreatedAt:    offer.CreatedAt,
				IsDisabled:   offer.IsDisabled,
			}

			if offer.Description != "" {
				params.Description = sql.NullString{
					String: offer.Description,
					Valid:  true,
				}
			}

			if offer.HasAmount {
				params.AmountMsat = sql.NullInt64{
					Int64: int64(offer.AmountMsat),
					Valid: true,
				}
			}

			if offer.Currency != "" {
				params.Currency = sql.NullString{
					String: offer.Currency,
					Valid:  true,
				}
			}

			if offer.HasExpiry {
				params.AbsoluteExpiry = sql.NullInt64{
					Int64: int64(offer.AbsoluteExpiry),
					Valid: true,
				}
			}

			if offer.HasQuantityMax {
				params.QuantityMax = sql.NullInt64{
					Int64: int64(offer.QuantityMax),
					Valid: true,
				}
			}

			var err error
			id, err = q.InsertOffer(ctx, params)

			return err
		},
		sqldb.NoOpReset,
	)
	if err != nil {
		return 0, fmt.Errorf("insert offer: %w", err)
	}

	return id, nil
}

// GetOffer retrieves an offer by its database ID.
func (s *SQLStore) GetOffer(ctx context.Context, id int64) (*Offer, error) {

	var offer *Offer

	err := s.db.ExecTx(
		ctx, sqldb.ReadTxOpt(),
		func(q SQLOfferQueries) error {
			row, err := q.GetOfferByID(ctx, id)
			if err != nil {
				return err
			}

			offer = sqlcOfferToOffer(row)

			return nil
		},
		sqldb.NoOpReset,
	)
	if err != nil {
		return nil, fmt.Errorf("get offer: %w", err)
	}

	return offer, nil
}

// GetOfferByOfferID retrieves an offer by its 32-byte offer ID hash.
func (s *SQLStore) GetOfferByOfferID(ctx context.Context, offerID [32]byte) (
	*Offer, error) {

	var offer *Offer

	err := s.db.ExecTx(
		ctx, sqldb.ReadTxOpt(),
		func(q SQLOfferQueries) error {
			row, err := q.GetOfferByOfferID(ctx, offerID[:])
			if err != nil {
				return err
			}

			offer = sqlcOfferToOffer(row)

			return nil
		},
		sqldb.NoOpReset,
	)
	if err != nil {
		return nil, fmt.Errorf("get offer by offer_id: %w", err)
	}

	return offer, nil
}

// ListOffers returns offers ordered by creation time. When activeOnly is true,
// disabled offers are excluded.
func (s *SQLStore) ListOffers(ctx context.Context, activeOnly bool) ([]*Offer,
	error) {

	var result []*Offer

	err := s.db.ExecTx(
		ctx, sqldb.ReadTxOpt(),
		func(q SQLOfferQueries) error {
			rows, err := q.ListOffers(
				ctx, sqlc.ListOffersParams{
					ActiveOnly: activeOnly,
					NumLimit:   1000,
					NumOffset:  0,
				},
			)
			if err != nil {
				return err
			}

			result = make([]*Offer, 0, len(rows))
			for _, row := range rows {
				result = append(
					result, sqlcOfferToOffer(row),
				)
			}

			return nil
		},
		sqldb.NoOpReset,
	)
	if err != nil {
		return nil, fmt.Errorf("list offers: %w", err)
	}

	return result, nil
}

// DisableOffer marks an offer as disabled.
func (s *SQLStore) DisableOffer(ctx context.Context, id int64) error {
	return s.db.ExecTx(
		ctx, sqldb.WriteTxOpt(),
		func(q SQLOfferQueries) error {
			result, err := q.DisableOffer(ctx, id)
			if err != nil {
				return err
			}

			rows, err := result.RowsAffected()
			if err != nil {
				return err
			}

			if rows == 0 {
				return fmt.Errorf("offer %d not found", id)
			}

			return nil
		},
		sqldb.NoOpReset,
	)
}

// sqlcOfferToOffer converts a sqlc.Offer row to our domain Offer type.
func sqlcOfferToOffer(row sqlc.Offer) *Offer {
	offer := &Offer{
		ID:         row.ID,
		Encoded:    row.Encoded,
		IsDisabled: row.IsDisabled,
		CreatedAt:  row.CreatedAt,
	}

	copy(offer.OfferID[:], row.OfferID)
	copy(offer.IssuerNodeID[:], row.IssuerNodeID)

	if row.Description.Valid {
		offer.Description = row.Description.String
	}

	if row.AmountMsat.Valid {
		offer.AmountMsat = uint64(row.AmountMsat.Int64)
		offer.HasAmount = true
	}

	if row.Currency.Valid {
		offer.Currency = row.Currency.String
	}

	if row.AbsoluteExpiry.Valid {
		offer.AbsoluteExpiry = uint64(row.AbsoluteExpiry.Int64)
		offer.HasExpiry = true
	}

	if row.QuantityMax.Valid {
		offer.QuantityMax = uint64(row.QuantityMax.Int64)
		offer.HasQuantityMax = true
	}

	return offer
}
