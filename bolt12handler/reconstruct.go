package bolt12handler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/invoices"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/offers"
	"github.com/lightningnetwork/lnd/tlv"
)

// Reconstructor implements invoices.Bolt12Reconstructor by decoding a signed
// envelope, verifying it, and building an invoices.Invoice from the offer
// store.
type Reconstructor struct {
	// nodePub is the node key that signed the envelopes.
	nodePub    *btcec.PublicKey
	offerStore offers.Store

	// activeChain is the genesis hash of the chain of this node. The
	// offer reader checks the stored offer against it.
	activeChain [32]byte

	// finalCLTVDelta is the final CLTV delta of the invoices, see
	// InvoiceConfig. The invoice does not carry it, so a changed config
	// value also applies to the invoices that are already out.
	finalCLTVDelta uint16
}

// NewReconstructor creates a Bolt12Reconstructor that verifies envelopes
// against nodePub and reads offers from the offer store.
func NewReconstructor(nodePub *btcec.PublicKey, offerStore offers.Store,
	activeChain [32]byte, finalCLTVDelta uint16) *Reconstructor {

	return &Reconstructor{
		nodePub:        nodePub,
		offerStore:     offerStore,
		activeChain:    activeChain,
		finalCLTVDelta: finalCLTVDelta,
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
	data, err := VerifyEnvelope(r.nodePub, signed)
	if err != nil {
		return nil, fmt.Errorf("verify envelope: %w", err)
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
		Memo:         r.offerMemo(offer, data.CreatedAt),
		CreationDate: time.Unix(int64(data.CreatedAt), 0).UTC(),
		Terms: invoices.ContractTerm{
			Expiry:          bolt12.DefaultInvoiceRelativeExpiry,
			FinalCltvDelta:  int32(r.finalCLTVDelta),
			PaymentPreimage: &preimage,
			PaymentAddr:     pathID,
			Value:           lnwire.MilliSatoshi(data.Amount),
			Features:        lnwire.EmptyFeatureVector(),
		},
		IsBolt12:       true,
		OfferID:        fn.Some(offer.ID),
		OfferHash:      offer.Hash[:],
		InvreqPayerID:  data.PayerID[:],
		InvreqQuantity: data.Quantity,
	}

	return invoice, nil
}

// offerMemo returns the offer_description of the stored offer, which is also
// the memo of the Open event. The offer is read at the creation time of the
// invoice, because the handler checked the offer expiry at that time, and an
// offer that expired since then must not fail the payment. When the offer
// cannot be read, the memo is empty.
func (r *Reconstructor) offerMemo(offer *offers.Offer,
	createdAt uint64) []byte {

	decoded, err := bolt12.DecodeOfferString(
		offer.Encoded, time.Unix(int64(createdAt), 0), r.activeChain,
	)
	if err != nil {
		log.Debugf("No memo for the BOLT 12 invoice of offer %x: %v",
			offer.Hash[:], err)

		return nil
	}

	var memo []byte
	decoded.OfferDescription.WhenSome(
		func(rec tlv.RecordT[tlv.TlvType10, tlv.Blob]) {
			memo = rec.Val
		},
	)

	return truncateMemo(memo)
}

// truncateMemo cuts the memo to invoices.MaxMemoSize bytes, because the
// invoice store refuses a longer memo. An onion message can carry a longer
// offer_description. The cut does not split a UTF-8 rune.
func truncateMemo(memo []byte) []byte {
	if len(memo) <= invoices.MaxMemoSize {
		return memo
	}

	cut := invoices.MaxMemoSize
	for cut > 0 && !utf8.RuneStart(memo[cut]) {
		cut--
	}

	return memo[:cut]
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
