package bolt12handler

import (
	"context"
	"time"
)

// InvReqRecord represents a persisted outgoing BOLT 12 invoice
// request/response pair.
type InvReqRecord struct {
	// ID is the database primary key.
	ID int64

	// OfferID is the 32-byte SHA256 hash of the TLV-encoded offer.
	OfferID []byte

	// InvReqBytes is the full TLV-encoded invoice request (includes
	// payer signature).
	InvReqBytes []byte

	// InvoiceBytes is the full TLV-encoded BOLT 12 invoice received
	// in response. Nil if the invoice has not been received yet.
	InvoiceBytes []byte

	// PayerKey is the 32-byte ephemeral private key scalar for
	// invreq_payer_id.
	PayerKey []byte

	// CreatedAt is when this negotiation was initiated.
	CreatedAt time.Time
}

// InvReqStore defines the interface for persisting and querying
// outgoing BOLT 12 invoice request/response pairs.
type InvReqStore interface {
	// Save persists a complete negotiation record (request + invoice
	// + payer key) keyed by offer ID.
	Save(ctx context.Context, offerID, invreqBytes,
		invoiceBytes, payerKey []byte) error

	// FetchByOfferID returns all negotiation records for the given
	// offer, ordered by creation time descending.
	FetchByOfferID(ctx context.Context,
		offerID []byte) ([]*InvReqRecord, error)
}
