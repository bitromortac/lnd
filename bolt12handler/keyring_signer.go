package bolt12handler

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/keychain"
)

// KeyRing is the part of the key ring that KeyRingSigner needs.
type KeyRing interface {
	keychain.MessageSignerRing
	keychain.ECDHRing
}

// KeyRingSigner implements Signer for the daemon. It signs and derives shared
// secrets through the key ring and never sees the private key, so it also
// works in remote signing mode.
type KeyRingSigner struct {
	keyRing KeyRing
	keyLoc  keychain.KeyLocator
	nodePub *btcec.PublicKey
}

// NewKeyRingSigner creates a Signer backed by the daemon's key ring.
func NewKeyRingSigner(keyRing KeyRing, keyLoc keychain.KeyLocator,
	nodePub *btcec.PublicKey) *KeyRingSigner {

	return &KeyRingSigner{
		keyRing: keyRing,
		keyLoc:  keyLoc,
		nodePub: nodePub,
	}
}

// NodePubKey returns the node identity public key.
//
// NOTE: This is part of the Signer interface.
func (s *KeyRingSigner) NodePubKey() *btcec.PublicKey {
	return s.nodePub
}

// PubKey returns the public key of key.
//
// NOTE: This is part of the Signer interface.
func (s *KeyRingSigner) PubKey(key KeyRef) (*btcec.PublicKey, error) {
	return tweakPubKey(s.nodePub, key)
}

// SignSchnorr signs the tagged hash of msg with key through the key ring.
//
// NOTE: This is part of the Signer interface.
func (s *KeyRingSigner) SignSchnorr(key KeyRef, tag string,
	msg []byte) ([64]byte, error) {

	sig, err := s.keyRing.SignMessageSchnorr(
		s.keyLoc, msg, false, key.AddTweak, key.MulTweak, []byte(tag),
	)
	if err != nil {
		return [64]byte{}, fmt.Errorf("sign %s: %w", tag, err)
	}

	return [64]byte(sig.Serialize()), nil
}

// ECDH returns the shared secret of the node key and pub.
//
// NOTE: This is part of the Signer interface.
func (s *KeyRingSigner) ECDH(pub *btcec.PublicKey) ([32]byte, error) {
	return s.keyRing.ECDH(keychain.KeyDescriptor{
		KeyLocator: s.keyLoc,
		PubKey:     s.nodePub,
	}, pub)
}
