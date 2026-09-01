package bolt12handler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/invoices"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/offers"
)

// Reconstructor implements invoices.Bolt12Reconstructor by decoding a signed
// envelope, verifying it, and building an invoices.Invoice from the offer
// store.
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

// ReconstructInvoice rebuilds a fully populated Invoice from a verified
// signed envelope and the stored offer that backs it.
//
// NOTE: This is part of the invoices.Bolt12Reconstructor interface.
func (r *Reconstructor) ReconstructInvoice(ctx context.Context,
	envelopeBytes []byte, pathID chainhash.Hash,
	paymentHash lntypes.Hash) (*invoices.Invoice, error) {

	signed, err := DecodeSignedEnvelope(envelopeBytes)
	if err != nil {
		return nil, fmt.Errorf("decode signed envelope: %w", err)
	}

	// Verify the tagged-hash signature using the node's public key.
	if err := r.signer.VerifyEnvelopeData(
		signed.OfferHash, signed.TLVData, signed.Signature,
	); err != nil {
		return nil, fmt.Errorf("verify envelope: %w", err)
	}

	data, err := DecodeEnvelopeData(signed.TLVData)
	if err != nil {
		return nil, fmt.Errorf("decode envelope data: %w", err)
	}

	computedHash := sha256.Sum256(data.Preimage[:])
	if lntypes.Hash(computedHash) != paymentHash {
		return nil, fmt.Errorf("preimage hash mismatch: "+
			"computed %x, expected %x",
			computedHash[:], paymentHash[:])
	}

	// The invoice expires after its last second, as the payer's codec
	// reads invoice_relative_expiry, so a payment in that second settles.
	relExpiry := uint64(bolt12.DefaultInvoiceRelativeExpiry / time.Second)
	expiryTime := data.CreatedAt + relExpiry
	now := uint64(time.Now().Unix())
	if now > expiryTime {
		return nil, fmt.Errorf("envelope expired: created_at=%d, "+
			"expiry=%d, now=%d",
			data.CreatedAt, expiryTime, now)
	}

	offer, err := r.offerStore.GetOfferByHash(
		ctx, signed.OfferHash,
	)
	if err != nil {
		return nil, fmt.Errorf("lookup offer: %w", err)
	}

	// A disabled offer takes no new payments. This runs only for the
	// first HTLC of an invoice, and no HTLC settles before the whole set
	// is accepted, so refusing here settles nothing for the offer.
	if offer.IsDisabled {
		return nil, ErrOfferDisabled
	}

	preimage := lntypes.Preimage(data.Preimage)

	invoice := &invoices.Invoice{
		CreationDate: time.Unix(int64(data.CreatedAt), 0).UTC(),
		Terms: invoices.ContractTerm{
			Expiry:          bolt12.DefaultInvoiceRelativeExpiry,
			FinalCltvDelta:  FinalCLTVDelta,
			PaymentPreimage: &preimage,
			PaymentAddr:     pathID,
			Value:           lnwire.MilliSatoshi(data.Amount),
			Features:        lnwire.EmptyFeatureVector(),
		},
		IsBolt12:       true,
		OfferID:        &offer.ID,
		OfferHash:      offer.Hash[:],
		InvreqPayerID:  data.PayerID[:],
		InvreqQuantity: data.Quantity,
	}

	return invoice, nil
}

// CheckOfferActive returns an error when the offer with the given hash is
// unknown or disabled.
//
// NOTE: This is part of the invoices.Bolt12Reconstructor interface.
func (r *Reconstructor) CheckOfferActive(ctx context.Context,
	offerHash []byte) error {

	if len(offerHash) != 32 {
		return fmt.Errorf("invalid offer hash length %d",
			len(offerHash))
	}

	offer, err := r.offerStore.GetOfferByHash(ctx, [32]byte(offerHash))
	if err != nil {
		return fmt.Errorf("lookup offer: %w", err)
	}

	if offer.IsDisabled {
		return ErrOfferDisabled
	}

	return nil
}

// Compile-time check that Reconstructor implements
// invoices.Bolt12Reconstructor.
var _ invoices.Bolt12Reconstructor = (*Reconstructor)(nil)
