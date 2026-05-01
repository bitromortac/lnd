package bolt12

import (
	"errors"
	"fmt"
	"time"
)

// maxOfferTLVBytes caps the raw TLV byte length accepted by DecodeOfferString.
// Real offers fit in a QR code (kilobyte-range), so this cap is many orders of
// magnitude above any legitimate offer while bounding allocation cost on
// malformed input from untrusted callers.
const maxOfferTLVBytes = 65535

// ErrOfferTooLarge is returned when an offer's TLV byte length exceeds
// maxOfferTLVBytes.
var ErrOfferTooLarge = errors.New("offer exceeds maximum size")

// DecodeOfferString decodes a BOLT 12 offer from its bech32 string
// representation (lno1...) and parses the TLV stream. now and activeChain
// plumb the spec's reader rules into the wrapper: offer_chains is gated
// against activeChain, and offer_absolute_expiry against now. The wrapper
// refuses to return an offer that would fail any MUST reader gate, so a
// Layer 2 caller cannot accidentally pay an expired or wrong-chain offer
// by skipping a follow-up validate step.
func DecodeOfferString(s string, now time.Time,
	activeChain [32]byte) (*Offer, error) {

	hrp, tlvBytes, err := Decode(s)
	if err != nil {
		return nil, fmt.Errorf("bech32: %w", err)
	}

	if hrp != HRPOffer {
		return nil, fmt.Errorf("expected HRP %q, got %q",
			HRPOffer, hrp)
	}

	if len(tlvBytes) > maxOfferTLVBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrOfferTooLarge,
			len(tlvBytes))
	}

	offer, err := decodeOffer(tlvBytes)
	if err != nil {
		return nil, err
	}

	if err := ValidateOfferRead(offer, now, activeChain, nil); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	return offer, nil
}

// EncodeOfferString encodes an offer to a TLV stream and returns the
// bech32 string (lno1...). Writer-side validation is delegated to
// (*Offer).Encode.
func EncodeOfferString(o *Offer) (string, error) {
	tlvBytes, err := o.Encode()
	if err != nil {
		return "", err
	}

	return Encode(HRPOffer, tlvBytes)
}

// DecodeInvoiceRequestString decodes a BOLT 12 invoice request from its
// bech32 string representation (lnr1...) and parses the TLV stream.
// activeChain is the genesis hash the receiver is willing to settle on;
// it gates the spec invreq_chain rule.
func DecodeInvoiceRequestString(s string,
	activeChain [32]byte) (*InvoiceRequest, error) {

	hrp, tlvBytes, err := Decode(s)
	if err != nil {
		return nil, fmt.Errorf("bech32: %w", err)
	}

	if hrp != HRPInvoiceRequest {
		return nil, fmt.Errorf("expected HRP %q, got %q",
			HRPInvoiceRequest, hrp)
	}

	ir, err := DecodeInvoiceRequest(tlvBytes)
	if err != nil {
		return nil, err
	}

	if err := ValidateInvoiceRequestRead(ir, activeChain, nil); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	return ir, nil
}

// EncodeInvoiceRequestString encodes an invoice request to a TLV stream
// and returns the bech32 string (lnr1...). The string form is only
// meaningful for transmission, so a populated signature is required:
// pre-sign Encode (used to compute the Merkle root) is permitted on
// (*InvoiceRequest).Encode, not at the wire-string layer. Writer-side
// validation is delegated to (*InvoiceRequest).Encode.
func EncodeInvoiceRequestString(ir *InvoiceRequest) (string, error) {
	if !ir.Signature.IsSome() {
		return "", ErrMissingSignature
	}

	tlvBytes, err := ir.Encode()
	if err != nil {
		return "", err
	}

	return Encode(HRPInvoiceRequest, tlvBytes)
}

// DecodeInvoiceString decodes a BOLT 12 invoice from its bech32 string
// representation (lni1...) and parses the TLV stream. now and
// activeChain plumb the spec's time-and-chain reader rules into the
// wrapper: invoice_relative_expiry (default 7200s) is enforced via
// ValidateInvoiceExpiry, and the invreq_chain rule via
// ValidateInvoiceRead. The wrapper refuses to return an invoice that
// would fail either MUST gate, so a Layer 2 caller cannot accidentally
// settle an expired or wrong-chain invoice by skipping a follow-up
// validate step.
func DecodeInvoiceString(s string, now time.Time,
	activeChain [32]byte) (*Invoice, error) {

	hrp, tlvBytes, err := Decode(s)
	if err != nil {
		return nil, fmt.Errorf("bech32: %w", err)
	}

	if hrp != HRPInvoice {
		return nil, fmt.Errorf("expected HRP %q, got %q",
			HRPInvoice, hrp)
	}

	inv, err := DecodeInvoice(tlvBytes)
	if err != nil {
		return nil, err
	}

	features := InvoiceFeatureCatalogues{
		Invoice: Bolt12Features,
		Blinded: Bolt12Features,
	}
	if err := ValidateInvoiceRead(inv, activeChain, features); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	if err := ValidateInvoiceExpiry(inv, now); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	return inv, nil
}

// EncodeInvoiceString encodes an invoice to a TLV stream and returns
// the bech32 string (lni1...). The string form is only meaningful for
// transmission, so a populated signature is required: pre-sign Encode
// (used to compute the Merkle root) is permitted on (*Invoice).Encode,
// not at the wire-string layer. Writer-side validation is delegated to
// (*Invoice).Encode.
func EncodeInvoiceString(inv *Invoice) (string, error) {
	if !inv.Signature.IsSome() {
		return "", ErrMissingSignature
	}

	tlvBytes, err := inv.Encode()
	if err != nil {
		return "", err
	}

	return Encode(HRPInvoice, tlvBytes)
}
