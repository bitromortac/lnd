package offers

import (
	"context"
	"time"
)

// Offer represents a persisted BOLT 12 offer.
type Offer struct {
	// ID is the database primary key.
	ID int64

	// OfferID is the SHA256 hash of the TLV-encoded offer, used as a unique
	// external identifier.
	OfferID [32]byte

	// Encoded is the full bech32-encoded offer string (lno1...).
	Encoded string

	// IssuerNodeID is the 33-byte compressed public key of the offer
	// issuer.
	IssuerNodeID [33]byte

	// Description is a UTF-8 description of the payment purpose.
	Description string

	// AmountMsat is the per-item amount in millisatoshis. Zero when the
	// offer uses a non-lightning currency or has no fixed amount.
	AmountMsat uint64

	// HasAmount indicates whether the offer specifies a fixed amount.
	HasAmount bool

	// Currency is the ISO 4217 currency code when the amount is not in
	// millisatoshis. Empty for msat-denominated offers.
	Currency string

	// AbsoluteExpiry is seconds since epoch after which the offer should
	// not be used. Zero means no expiry.
	AbsoluteExpiry uint64

	// HasExpiry indicates whether the offer has an explicit expiry.
	HasExpiry bool

	// QuantityMax is the maximum items per invoice. Zero means unlimited.
	// Only valid when HasQuantityMax is true.
	QuantityMax uint64

	// HasQuantityMax indicates whether the offer supports quantity.
	HasQuantityMax bool

	// IsDisabled indicates the offer has been administratively disabled.
	IsDisabled bool

	// CreatedAt is the time the offer was created.
	CreatedAt time.Time
}

// Store defines the interface for persisting and querying BOLT 12 offers.
type Store interface {
	// InsertOffer persists a new offer and returns its database ID.
	InsertOffer(ctx context.Context, offer *Offer) (int64, error)

	// GetOffer retrieves an offer by its database ID.
	GetOffer(ctx context.Context, id int64) (*Offer, error)

	// GetOfferByOfferID retrieves an offer by its 32-byte offer ID hash.
	GetOfferByOfferID(ctx context.Context, offerID [32]byte) (*Offer, error)

	// ListOffers returns offers ordered by creation time. When activeOnly
	// is true, disabled offers are excluded.
	ListOffers(ctx context.Context, activeOnly bool) ([]*Offer, error)

	// DisableOffer marks an offer as disabled so it rejects new invoice
	// requests while allowing in-flight invoices to settle.
	DisableOffer(ctx context.Context, id int64) error
}
