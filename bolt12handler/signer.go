package bolt12handler

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
)

// NodeSigner provides the node's identity key for BOLT 12 invoice signing and
// envelope operations.
type NodeSigner interface {
	// NodePubKey returns the node's identity public key.
	NodePubKey() *btcec.PublicKey

	// SignInvoice signs a BOLT 12 invoice with a BIP-340 Schnorr signature
	// from the node's identity private key.
	SignInvoice(inv *bolt12.Invoice) ([64]byte, error)

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
