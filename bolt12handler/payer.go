package bolt12handler

import (
	"crypto/rand"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/tlv"
)

// Payer is the identity that signs an invoice request: a key of a signer and
// the invreq_metadata of the request. invreq_payer_id is the public key of
// Key.
type Payer struct {
	// Signer holds the payer key.
	Signer Signer

	// Key names the payer key of Signer.
	Key KeyRef

	// Metadata is the invreq_metadata of the request.
	Metadata []byte
}

// NewRandomPayer returns a payer with a fresh random key and metadata. The
// node stores neither, so a retry builds another request.
func NewRandomPayer() (*Payer, error) {
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("generate payer key: %w", err)
	}

	metadata := make([]byte, 32)
	if _, err := rand.Read(metadata); err != nil {
		return nil, fmt.Errorf("generate metadata: %w", err)
	}

	return &Payer{
		Signer:   NewPrivKeySigner(privKey),
		Metadata: metadata,
	}, nil
}

// PubKey returns invreq_payer_id.
func (p *Payer) PubKey() (*btcec.PublicKey, error) {
	return p.Signer.PubKey(p.Key)
}

// signInvoiceRequest signs ir with the payer key and attaches the signature
// in place. The root function runs the writer validation first, so a
// malformed request fails before the key signs it.
func (p *Payer) signInvoiceRequest(ir *bolt12.InvoiceRequest) error {
	root, err := bolt12.InvoiceRequestSigningRoot(ir)
	if err != nil {
		return err
	}

	sig, err := p.Signer.SignSchnorr(
		p.Key, bolt12.InvoiceRequestSignatureTag, root[:],
	)
	if err != nil {
		return fmt.Errorf("sign invoice request: %w", err)
	}

	ir.Signature = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType240, [64]byte](sig),
	)

	return nil
}
