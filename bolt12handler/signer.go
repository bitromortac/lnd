package bolt12handler

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
)

// NodeSigner provides the node's identity key for BOLT 12 invoice signing.
type NodeSigner interface {
	// NodePubKey returns the node's identity public key.
	NodePubKey() *btcec.PublicKey

	// SignInvoice signs a BOLT 12 invoice using the node's identity private
	// key and returns the 64-byte Schnorr signature.
	SignInvoice(inv *bolt12.Invoice) ([64]byte, error)
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
