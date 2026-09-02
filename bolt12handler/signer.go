package bolt12handler

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/lightningnetwork/lnd/bolt12"
)

// NodeSigner provides the node's identity key for BOLT 12 invoice signing and
// envelope operations.
type NodeSigner interface {
	// NodePubKey returns the node's identity public key.
	NodePubKey() *btcec.PublicKey

	// SignInvoice signs a BOLT 12 invoice using the node's identity private
	// key and returns the 64-byte Schnorr signature.
	SignInvoice(inv *bolt12.Invoice) ([64]byte, error)

	// BlindedNodePubKey returns the blinded_node_id this node was given
	// for pathKey, the route-blinding ephemeral key an onion message
	// arrived under. An offer that publishes offer_paths rather than
	// offer_issuer_id binds invoice_node_id to this key.
	BlindedNodePubKey(pathKey *btcec.PublicKey) (*btcec.PublicKey, error)

	// SignInvoiceBlinded signs a BOLT 12 invoice with the blinded node
	// key for pathKey and returns the 64-byte Schnorr signature.
	SignInvoiceBlinded(inv *bolt12.Invoice,
		pathKey *btcec.PublicKey) ([64]byte, error)

	// SignEnvelopeData signs envelope data using a BIP-340 tagged hash:
	// tagged_hash("bolt12/envelope", offerHash || data). Returns the
	// 64-byte Schnorr signature.
	SignEnvelopeData(offerHash [32]byte,
		data []byte) ([64]byte, error)

	// VerifyEnvelopeData verifies a tagged-hash signature over envelope
	// data using the node's public key.
	VerifyEnvelopeData(offerHash [32]byte,
		data []byte, sig [64]byte) error
}

// PrivKeySigner implements NodeSigner using a raw private key. This is used in
// tests and in environments where the private key is directly available.
type PrivKeySigner struct {
	privKey *btcec.PrivateKey
}

// NewPrivKeySigner creates a NodeSigner backed by a raw private key.
func NewPrivKeySigner(privKey *btcec.PrivateKey) *PrivKeySigner {
	return &PrivKeySigner{privKey: privKey}
}

// NodePubKey returns the node's identity public key.
//
// NOTE: This is part of the NodeSigner interface.
func (s *PrivKeySigner) NodePubKey() *btcec.PublicKey {
	return s.privKey.PubKey()
}

// SignInvoice signs a BOLT 12 invoice using the wrapped private key.
//
// NOTE: This is part of the NodeSigner interface.
func (s *PrivKeySigner) SignInvoice(inv *bolt12.Invoice) ([64]byte, error) {
	return bolt12.SignInvoice(inv, s.privKey)
}

// BlindedNodePubKey returns the blinded_node_id derived for pathKey.
//
// NOTE: This is part of the NodeSigner interface.
func (s *PrivKeySigner) BlindedNodePubKey(
	pathKey *btcec.PublicKey) (*btcec.PublicKey, error) {

	blindedKey, err := blindPrivKey(s.privKey, pathKey)
	if err != nil {
		return nil, err
	}

	return blindedKey.PubKey(), nil
}

// SignInvoiceBlinded signs a BOLT 12 invoice with the blinded node key for
// pathKey.
//
// NOTE: This is part of the NodeSigner interface.
func (s *PrivKeySigner) SignInvoiceBlinded(inv *bolt12.Invoice,
	pathKey *btcec.PublicKey) ([64]byte, error) {

	blindedKey, err := blindPrivKey(s.privKey, pathKey)
	if err != nil {
		return [64]byte{}, err
	}

	return bolt12.SignInvoice(inv, blindedKey)
}

// SignEnvelopeData signs envelope data using a BIP-340 tagged hash:
// tagged_hash("bolt12/envelope", offerHash || data).
//
// NOTE: This is part of the NodeSigner interface.
func (s *PrivKeySigner) SignEnvelopeData(offerHash [32]byte,
	data []byte) ([64]byte, error) {

	digest := envelopeDigest(offerHash, data)

	sig, err := schnorr.Sign(s.privKey, digest[:])
	if err != nil {
		return [64]byte{}, fmt.Errorf("sign envelope: %w", err)
	}

	var result [64]byte
	copy(result[:], sig.Serialize())

	return result, nil
}

// VerifyEnvelopeData verifies a tagged-hash signature over envelope data using
// the node's public key.
//
// NOTE: This is part of the NodeSigner interface.
func (s *PrivKeySigner) VerifyEnvelopeData(offerHash [32]byte,
	data []byte, sig [64]byte) error {

	digest := envelopeDigest(offerHash, data)

	parsedSig, err := schnorr.ParseSignature(sig[:])
	if err != nil {
		return fmt.Errorf("parse signature: %w", err)
	}

	if !parsedSig.Verify(digest[:], s.privKey.PubKey()) {
		return fmt.Errorf("envelope signature verification failed")
	}

	return nil
}
