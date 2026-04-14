package offers

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/tlv"
)

var (
	// ErrMissingDescription is returned when the offer has an amount but no
	// description.
	ErrMissingDescription = errors.New("description required when amount " +
		"is set")

	// ErrMissingIssuerKey is returned when no issuer public key is
	// provided.
	ErrMissingIssuerKey = errors.New("issuer public key required")
)

// CreateOfferParams contains the parameters for creating a new offer.
type CreateOfferParams struct {
	// Identity specifies how the receiver is identified in the offer.
	// Left: offer_issuer_id (public key, reveals node identity).
	// Right: offer_paths (blinded message paths, preserves privacy).
	Identity fn.Either[*btcec.PublicKey, []lnwire.BlindedPath]

	// Description is the UTF-8 description of the payment purpose. Required
	// when Amount is set.
	Description string

	// AmountMsat is the per-item amount in millisatoshis. Zero means no
	// fixed amount (the payer must specify invreq_amount).
	AmountMsat uint64

	// AbsoluteExpiry is seconds since epoch after which the offer expires.
	// Zero means no expiry.
	AbsoluteExpiry uint64

	// QuantityMax is the maximum items per invoice. Zero means the offer
	// does not support quantity selection. A non-zero value enables
	// quantity (with 0 stored to mean unlimited per the spec).
	QuantityMax *uint64

	// Chains specifies which blockchain networks this offer is valid for.
	// When non-empty, offer_chains (type 2) is set. When empty, the spec
	// defaults to Bitcoin mainnet — so non-mainnet offers MUST set this.
	Chains [][32]byte
}

// CreateOfferResult contains the result of creating a new offer.
type CreateOfferResult struct {
	// ID is the database primary key of the newly created offer.
	ID int64

	// OfferID is the SHA256 hash of the TLV-encoded offer.
	OfferID [32]byte

	// Encoded is the bech32-encoded offer string (lno1...).
	Encoded string
}

// CreateOffer validates input, constructs a BOLT 12 offer, encodes it, computes
// the offer ID, persists it to the store, and returns the result.
func CreateOffer(ctx context.Context, store Store,
	params CreateOfferParams) (*CreateOfferResult, error) {

	// Validate the identity parameter.
	var identityErr error
	params.Identity.WhenLeft(func(key *btcec.PublicKey) {
		if key == nil {
			identityErr = ErrMissingIssuerKey
		}
	})
	params.Identity.WhenRight(func(paths []lnwire.BlindedPath) {
		if len(paths) == 0 {
			identityErr = fmt.Errorf("offer_paths must " +
				"contain at least one path")
		}
	})
	if identityErr != nil {
		return nil, identityErr
	}

	// The spec requires offer_description when offer_amount is set.
	if params.AmountMsat > 0 && params.Description == "" {
		return nil, ErrMissingDescription
	}

	// Build the bolt12 offer struct.
	offer := &bolt12.Offer{}

	// Set identity: either offer_issuer_id or offer_paths.
	params.Identity.WhenLeft(func(key *btcec.PublicKey) {
		offer.OfferIssuerID = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType22, *btcec.PublicKey]{
				Val: key,
			},
		)
	})
	params.Identity.WhenRight(func(paths []lnwire.BlindedPath) {
		offer.OfferPaths = tlv.SomeRecordT(
			tlv.RecordT[
				tlv.TlvType16, lnwire.BlindedPaths,
			]{
				Val: lnwire.BlindedPaths{
					Paths: paths,
				},
			},
		)
	})

	// Set offer_chains (type 2) if specified. Required for non-mainnet
	// networks since the spec defaults to Bitcoin mainnet when absent.
	if len(params.Chains) > 0 {
		offer.OfferChains = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType2, bolt12.ChainsRecord]{
				Val: bolt12.ChainsRecord{
					Chains: params.Chains,
				},
			},
		)
	}

	// Set offer_description (type 10) if provided.
	if params.Description != "" {
		offer.OfferDescription = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType10, tlv.Blob]{
				Val: []byte(params.Description),
			},
		)
	}

	// Set offer_amount (type 8) if non-zero.
	if params.AmountMsat > 0 {
		amount := bolt12.TUint64(params.AmountMsat)
		offer.OfferAmount = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType8, bolt12.TUint64]{
				Val: amount,
			},
		)
	}

	// Set offer_absolute_expiry (type 14) if non-zero.
	if params.AbsoluteExpiry > 0 {
		expiry := bolt12.TUint64(params.AbsoluteExpiry)
		offer.OfferAbsoluteExpiry = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType14, bolt12.TUint64]{
				Val: expiry,
			},
		)
	}

	// Set offer_quantity_max (type 20) if specified.
	if params.QuantityMax != nil {
		qty := bolt12.TUint64(*params.QuantityMax)
		offer.OfferQuantityMax = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType20, bolt12.TUint64]{
				Val: qty,
			},
		)
	}

	// Encode the offer to its bech32 string representation. This also runs
	// ValidateOfferWrite internally.
	encoded, err := bolt12.EncodeOfferString(offer)
	if err != nil {
		return nil, fmt.Errorf("encode offer: %w", err)
	}

	// Compute offer_id as SHA256 of the TLV-encoded offer bytes.
	tlvBytes, err := offer.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode offer TLV: %w", err)
	}
	offerID := sha256.Sum256(tlvBytes)

	// Build the store offer record.
	storeOffer := &Offer{
		OfferID:     offerID,
		Encoded:     encoded,
		Description: params.Description,
		CreatedAt:   time.Now().UTC(),
	}
	offer.OfferIssuerID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType22, *btcec.PublicKey]) {
			copy(
				storeOffer.IssuerNodeID[:],
				r.Val.SerializeCompressed(),
			)
		},
	)

	if params.AmountMsat > 0 {
		storeOffer.AmountMsat = params.AmountMsat
		storeOffer.HasAmount = true
	}

	if params.AbsoluteExpiry > 0 {
		storeOffer.AbsoluteExpiry = params.AbsoluteExpiry
		storeOffer.HasExpiry = true
	}

	if params.QuantityMax != nil {
		storeOffer.QuantityMax = *params.QuantityMax
		storeOffer.HasQuantityMax = true
	}

	// Persist to the store.
	id, err := store.InsertOffer(ctx, storeOffer)
	if err != nil {
		return nil, fmt.Errorf("persist offer: %w", err)
	}

	return &CreateOfferResult{
		ID:      id,
		OfferID: offerID,
		Encoded: encoded,
	}, nil
}
