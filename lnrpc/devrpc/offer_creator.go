package devrpc

import (
	"context"

	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/offers"
)

// OfferCreator creates and stores the BOLT 12 offers of this node.
type OfferCreator interface {
	// CreateOffer creates an offer, stores it, and returns its encoded
	// string and offer hash. The offer names the node's identity, or it
	// reaches the node over blinded paths when useBlindedPaths is set.
	CreateOffer(ctx context.Context, description string, amountMsat,
		absoluteExpiry uint64, quantityMax fn.Option[uint64],
		useBlindedPaths bool) (*offers.CreateOfferResult, error)
}
