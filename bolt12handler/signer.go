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
