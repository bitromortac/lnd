package bolt12handler

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/keychain"
)

// KeyRingSigner implements NodeSigner using a KeyRing and SingleKeyECDH. This
// is the production signer used inside the daemon where the raw private key is
// not directly accessible.
//
// For BOLT 12, we need the node's identity key to sign invoices using BIP-340
// Schnorr signatures. The KeyRing's DerivePrivKey method extracts the raw
// private key, which is then used for the Schnorr signature.
type KeyRingSigner struct {
	keyRing     keychain.SecretKeyRing
	keyLoc      keychain.KeyLocator
	identityPub *btcec.PublicKey
}

// NewKeyRingSigner creates a NodeSigner backed by the daemon's key ring.
func NewKeyRingSigner(keyRing keychain.SecretKeyRing,
	keyLoc keychain.KeyLocator,
	identityPub *btcec.PublicKey) *KeyRingSigner {

	return &KeyRingSigner{
		keyRing:     keyRing,
		keyLoc:      keyLoc,
		identityPub: identityPub,
	}
}

// NodePubKey returns the node's identity public key.
//
// NOTE: This is part of the NodeSigner interface.
func (s *KeyRingSigner) NodePubKey() *btcec.PublicKey {
	return s.identityPub
}

// SignInvoice signs a BOLT 12 invoice using the node's identity private key
// derived from the key ring.
//
// NOTE: This is part of the NodeSigner interface.
func (s *KeyRingSigner) SignInvoice(inv *bolt12.Invoice) ([64]byte, error) {

	privKey, err := s.derivePrivKey()
	if err != nil {
		return [64]byte{}, err
	}

	return bolt12.SignInvoice(inv, privKey)
}

// SignEnvelopeData signs envelope data using a BIP-340 tagged hash:
// tagged_hash("bolt12/envelope", offerIDHash || data).
//
// NOTE: This is part of the NodeSigner interface.
func (s *KeyRingSigner) SignEnvelopeData(offerIDHash [32]byte,
	data []byte) ([64]byte, error) {

	privKey, err := s.derivePrivKey()
	if err != nil {
		return [64]byte{}, err
	}

	digest := envelopeDigest(offerIDHash, data)

	sig, err := schnorr.Sign(privKey, digest[:])
	if err != nil {
		return [64]byte{}, fmt.Errorf("sign envelope: %w", err)
	}

	var result [64]byte
	copy(result[:], sig.Serialize())

	return result, nil
}

// VerifyEnvelopeData verifies a tagged-hash signature over envelope data using
// the node's identity public key.
//
// NOTE: This is part of the NodeSigner interface.
func (s *KeyRingSigner) VerifyEnvelopeData(offerIDHash [32]byte,
	data []byte, sig [64]byte) error {

	digest := envelopeDigest(offerIDHash, data)

	parsedSig, err := schnorr.ParseSignature(sig[:])
	if err != nil {
		return fmt.Errorf("parse signature: %w", err)
	}

	if !parsedSig.Verify(digest[:], s.identityPub) {
		return fmt.Errorf("envelope signature verification failed")
	}

	return nil
}

// derivePrivKey extracts the raw private key from the key ring.
func (s *KeyRingSigner) derivePrivKey() (*btcec.PrivateKey, error) {
	privKey, err := s.keyRing.DerivePrivKey(
		keychain.KeyDescriptor{
			KeyLocator: s.keyLoc,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("derive identity key: %w", err)
	}

	return privKey, nil
}
