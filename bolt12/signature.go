package bolt12

import (
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/lightningnetwork/lnd/tlv"
)

// signatureTagPrefix is the literal prefix for all BOLT 12 signature
// tags.
const signatureTagPrefix = "lightning"

// ErrInvalidSignature is returned by VerifyInvoice and
// VerifyInvoiceRequest when the BIP-340 Schnorr signature does not
// validate against the message's Merkle root and signing key.
var ErrInvalidSignature = errors.New("BOLT 12 signature is invalid")

// SignMessage creates a BIP-340 Schnorr signature over the Merkle root
// of a BOLT 12 message. The tag is "lightning" || messageName ||
// fieldName.
func SignMessage(
	messageName, fieldName string,
	merkleRoot [32]byte,
	privKey *btcec.PrivateKey) ([64]byte, error) {

	tag := signatureTagPrefix + messageName + fieldName
	digest := taggedHash(tag, merkleRoot[:])

	sig, err := schnorr.Sign(privKey, digest[:])
	if err != nil {
		return [64]byte{}, fmt.Errorf("sign: %w", err)
	}

	var result [64]byte
	copy(result[:], sig.Serialize())

	return result, nil
}

// VerifySignature verifies a BIP-340 Schnorr signature over the Merkle
// root of a BOLT 12 message.
func VerifySignature(
	messageName, fieldName string,
	merkleRoot [32]byte,
	sig [64]byte,
	pubKey *btcec.PublicKey) error {

	tag := signatureTagPrefix + messageName + fieldName
	digest := taggedHash(tag, merkleRoot[:])

	parsedSig, err := schnorr.ParseSignature(sig[:])
	if err != nil {
		return fmt.Errorf("parse signature: %w", err)
	}

	if !parsedSig.Verify(digest[:], pubKey) {
		return ErrInvalidSignature
	}

	return nil
}

// signableTLVs returns the subset of records that contribute to the
// signature's Merkle root. Everything outside the inclusive range
// [240, 1000] is included; types 240-1000 are reserved by the BOLT 12
// spec for the signature TLV (type 240) and similar non-content fields
// the signer must not commit to. The reserved range covers more than
// just signature, so the filter is symmetric on both ends rather than
// a single-type exclusion.
func signableTLVs(records []tlv.Record) []tlv.Record {
	out := make([]tlv.Record, 0, len(records))
	for _, r := range records {
		if !bolt12InUnsignedRange(r.Type()) {
			out = append(out, r)
		}
	}

	return out
}

// SignInvoiceRequest computes the Merkle root of an invoice request
// and generates a Schnorr signature using the provided private key.
// The root is computed over the signable subset of AllRecords(); the
// caller must ensure the typed fields reflect the desired final state
// before calling Sign — there is no implicit canonicalisation step.
func SignInvoiceRequest(ir *InvoiceRequest, privKey *btcec.PrivateKey) (
	[64]byte, error) {

	root, err := MerkleRoot(signableTLVs(ir.AllRecords()))
	if err != nil {
		return [64]byte{}, err
	}

	return SignMessage(
		"invoice_request", "signature", root, privKey,
	)
}

// VerifyInvoiceRequest verifies the signature on an invoice request using
// its invreq_payer_id public key.
func VerifyInvoiceRequest(ir *InvoiceRequest) error {
	var (
		pubKey     *btcec.PublicKey
		hasPayerID bool
	)
	ir.InvreqPayerID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType88, *btcec.PublicKey]) {
			pubKey = r.Val
			hasPayerID = true
		},
	)
	if !hasPayerID {
		return ErrMissingPayerID
	}
	if pubKey == nil {
		return fmt.Errorf("%w: invreq_payer_id", ErrInvalidPubKey)
	}

	var (
		sig    [64]byte
		hasSig bool
	)
	ir.Signature.WhenSome(
		func(r tlv.RecordT[tlv.TlvType240, [64]byte]) {
			sig = r.Val
			hasSig = true
		},
	)
	if !hasSig {
		return ErrMissingSignature
	}

	root, err := MerkleRoot(signableTLVs(ir.AllRecords()))
	if err != nil {
		return err
	}

	return VerifySignature(
		"invoice_request", "signature", root, sig, pubKey,
	)
}

// SignInvoice computes the Merkle root of an invoice and generates a
// Schnorr signature using the provided private key. The root is
// computed over the signable subset of AllRecords(); the caller must
// ensure the typed fields reflect the desired final state before
// calling Sign — there is no implicit canonicalisation step.
func SignInvoice(inv *Invoice, privKey *btcec.PrivateKey) ([64]byte, error) {
	root, err := MerkleRoot(signableTLVs(inv.AllRecords()))
	if err != nil {
		return [64]byte{}, err
	}

	return SignMessage("invoice", "signature", root, privKey)
}

// VerifyInvoice verifies the signature on an invoice using its
// invoice_node_id public key.
func VerifyInvoice(inv *Invoice) error {
	var (
		pubKey    *btcec.PublicKey
		hasNodeID bool
	)
	inv.InvoiceNodeID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType176, *btcec.PublicKey]) {
			pubKey = r.Val
			hasNodeID = true
		},
	)
	if !hasNodeID {
		return ErrMissingNodeID
	}
	if pubKey == nil {
		return fmt.Errorf("%w: invoice_node_id", ErrInvalidPubKey)
	}

	var (
		sig    [64]byte
		hasSig bool
	)
	inv.Signature.WhenSome(
		func(r tlv.RecordT[tlv.TlvType240, [64]byte]) {
			sig = r.Val
			hasSig = true
		},
	)
	if !hasSig {
		return ErrMissingSignature
	}

	root, err := MerkleRoot(signableTLVs(inv.AllRecords()))
	if err != nil {
		return err
	}

	return VerifySignature("invoice", "signature", root, sig, pubKey)
}
