package bolt12handler

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/lightningnetwork/lnd/bolt12"
)

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

// SignEnvelopeData signs envelope data using a BIP-340 tagged hash:
// tagged_hash("bolt12/envelope", offerIDHash || data).
//
// NOTE: This is part of the NodeSigner interface.
func (s *PrivKeySigner) SignEnvelopeData(offerIDHash [32]byte,
	data []byte) ([64]byte, error) {

	digest := envelopeDigest(offerIDHash, data)

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
func (s *PrivKeySigner) VerifyEnvelopeData(offerIDHash [32]byte,
	data []byte, sig [64]byte) error {

	digest := envelopeDigest(offerIDHash, data)

	parsedSig, err := schnorr.ParseSignature(sig[:])
	if err != nil {
		return fmt.Errorf("parse signature: %w", err)
	}

	if !parsedSig.Verify(digest[:], s.privKey.PubKey()) {
		return fmt.Errorf("envelope signature verification failed")
	}

	return nil
}
