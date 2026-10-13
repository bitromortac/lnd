package bolt12handler

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/lightningnetwork/lnd/keychain"
)

// PrivKeySigner implements Signer with a raw private key. A random payer key
// is not derived from the node key, so it signs in this way, and tests use it
// in place of the key ring.
type PrivKeySigner struct {
	privKey *btcec.PrivateKey
}

// NewPrivKeySigner creates a Signer backed by a raw private key.
func NewPrivKeySigner(privKey *btcec.PrivateKey) *PrivKeySigner {
	return &PrivKeySigner{privKey: privKey}
}

// NodePubKey returns the public key of the wrapped private key.
//
// NOTE: This is part of the Signer interface.
func (s *PrivKeySigner) NodePubKey() *btcec.PublicKey {
	return s.privKey.PubKey()
}

// PubKey returns the public key of key.
//
// NOTE: This is part of the Signer interface.
func (s *PrivKeySigner) PubKey(key KeyRef) (*btcec.PublicKey, error) {
	return tweakPubKey(s.privKey.PubKey(), key)
}

// SignSchnorr signs the tagged hash of msg with the tweaked private key.
//
// NOTE: This is part of the Signer interface.
func (s *PrivKeySigner) SignSchnorr(key KeyRef, tag string,
	msg []byte) ([64]byte, error) {

	privKey, err := tweakPrivKey(s.privKey, key)
	if err != nil {
		return [64]byte{}, err
	}

	digest := chainhash.TaggedHash([]byte(tag), msg)
	sig, err := schnorr.Sign(privKey, digest[:])
	if err != nil {
		return [64]byte{}, err
	}

	return [64]byte(sig.Serialize()), nil
}

// ECDH returns the shared secret of the private key and pub.
//
// NOTE: This is part of the Signer interface.
func (s *PrivKeySigner) ECDH(pub *btcec.PublicKey) ([32]byte, error) {
	return (&keychain.PrivKeyECDH{PrivKey: s.privKey}).ECDH(pub)
}
