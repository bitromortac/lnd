package bolt12handler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/invoices"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/offers"
)

const (
	// defaultRelativeExpiry is the default relative expiry for BOLT 12
	// invoices in seconds. Envelopes older than createdAt + this value are
	// rejected. Matches Eclair's default of 2 hours.
	defaultRelativeExpiry = 7200
)

// Reconstructor implements invoices.Bolt12Reconstructor by decoding a signed
// envelope, verifying it, and building an invoices.Invoice from the offer store.
type Reconstructor struct {
	signer     NodeSigner
	offerStore offers.Store
}

// NewReconstructor creates a Bolt12Reconstructor backed by the given signer and
// offer store.
func NewReconstructor(signer NodeSigner,
	offerStore offers.Store) *Reconstructor {

	return &Reconstructor{
		signer:     signer,
		offerStore: offerStore,
	}
}

// ReconstructInvoice decodes and verifies the signed envelope, validates the
// preimage against the payment hash, checks expiry, looks up the offer, and
// returns a fully populated Invoice struct ready for INSERT.
//
// NOTE: This is part of the invoices.Bolt12Reconstructor interface.
func (r *Reconstructor) ReconstructInvoice(ctx context.Context,
	envelopeBytes []byte, pathID chainhash.Hash,
	paymentHash lntypes.Hash) (*invoices.Invoice, error) {

	// Decode the wire-format signed envelope.
	signed, err := DecodeSignedEnvelope(envelopeBytes)
	if err != nil {
		return nil, fmt.Errorf("decode signed envelope: %w", err)
	}

	// Verify the tagged-hash signature using the node's public key.
	if err := r.signer.VerifyEnvelopeData(
		signed.OfferIDHash, signed.TLVData, signed.Signature,
	); err != nil {

		return nil, fmt.Errorf("verify envelope: %w", err)
	}

	// Decode the TLV data inside the envelope.
	data, err := DecodeEnvelopeData(signed.TLVData)
	if err != nil {
		return nil, fmt.Errorf("decode envelope data: %w", err)
	}

	// Verify sha256(preimage) == paymentHash.
	computedHash := sha256.Sum256(data.Preimage[:])
	if lntypes.Hash(computedHash) != paymentHash {
		return nil, fmt.Errorf("preimage hash mismatch: "+
			"computed %x, expected %x",
			computedHash[:], paymentHash[:])
	}

	// Verify invoice not expired: createdAt + relativeExpiry <= now.
	expiryTime := data.CreatedAt + defaultRelativeExpiry
	now := uint64(time.Now().Unix())
	if expiryTime <= now {
		return nil, fmt.Errorf("envelope expired: created_at=%d, "+
			"expiry=%d, now=%d",
			data.CreatedAt, expiryTime, now)
	}

	// Look up the offer by offer ID hash.
	offer, err := r.offerStore.GetOfferByOfferID(
		ctx, signed.OfferIDHash,
	)
	if err != nil {
		return nil, fmt.Errorf("lookup offer: %w", err)
	}

	// Build the invoice struct for INSERT.
	preimage := lntypes.Preimage(data.Preimage)

	invoice := &invoices.Invoice{
		CreationDate: time.Unix(int64(data.CreatedAt), 0).UTC(),
		Terms: invoices.ContractTerm{
			Expiry:          defaultRelativeExpiry * time.Second,
			PaymentPreimage: &preimage,
			PaymentAddr:     pathID,
			Value:           lnwire.MilliSatoshi(data.Amount),
			Features:        lnwire.EmptyFeatureVector(),
		},
		IsBolt12:    true,
		OfferID:     &offer.ID,
		OfferIDHash: offer.OfferID[:],
		InvoiceNodeID: r.signer.NodePubKey().
			SerializeCompressed(),
		InvreqPayerID: data.PayerID[:],
	}

	return invoice, nil
}

// Compile-time check that Reconstructor implements
// invoices.Bolt12Reconstructor.
var _ invoices.Bolt12Reconstructor = (*Reconstructor)(nil)
