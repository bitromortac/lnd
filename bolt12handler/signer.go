package bolt12handler

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/lightningnetwork/lnd/keychain"
)

// KeyRef names a key that a Signer derives from the node key. The zero value
// is the node key itself.
type KeyRef struct {
	// MulTweak is an optional 32-byte scalar that multiplies the node key,
	// as route blinding derives a blinded node key. It applies before
	// AddTweak.
	MulTweak []byte

	// AddTweak is an optional tweak that is applied as BIP 341 tweaks an
	// internal key, so the key becomes k + H_TapTweak(P || AddTweak).
	AddTweak []byte
}

// Signer signs BOLT 12 messages with the node key or a key derived from it.
// It never shows a private key, so a remote signer can implement it.
type Signer interface {
	// NodePubKey returns the node identity public key.
	NodePubKey() *btcec.PublicKey

	// PubKey returns the public key of key.
	PubKey(key KeyRef) (*btcec.PublicKey, error)

	// SignSchnorr returns a BIP-340 Schnorr signature by key over the
	// tagged hash of msg under tag.
	SignSchnorr(key KeyRef, tag string, msg []byte) ([64]byte, error)

	// ECDH returns the SHA256 of the shared point of the node key and pub,
	// as keychain.SingleKeyECDH does.
	ECDH(pub *btcec.PublicKey) ([32]byte, error)
}

// tweakPubKey applies the tweaks of key to the node public key. It is the
// public side of tweakPrivKey.
func tweakPubKey(nodePub *btcec.PublicKey,
	key KeyRef) (*btcec.PublicKey, error) {

	pub := nodePub

	if len(key.MulTweak) > 0 {
		var err error
		pub, err = keychain.MulTweakPubKey(pub, key.MulTweak)
		if err != nil {
			return nil, err
		}
	}

	if len(key.AddTweak) > 0 {
		pub = txscript.ComputeTaprootOutputKey(pub, key.AddTweak)
	}

	return pub, nil
}

// tweakPrivKey applies the tweaks of key to the node private key, in the
// same order as the key ring does.
func tweakPrivKey(nodeKey *btcec.PrivateKey,
	key KeyRef) (*btcec.PrivateKey, error) {

	privKey := nodeKey

	if len(key.MulTweak) > 0 {
		var err error
		privKey, err = keychain.MulTweakPrivKey(privKey, key.MulTweak)
		if err != nil {
			return nil, err
		}
	}

	if len(key.AddTweak) > 0 {
		privKey = txscript.TweakTaprootPrivKey(*privKey, key.AddTweak)
	}

	return privKey, nil
}
