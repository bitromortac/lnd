package bolt12handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
)

const (
	// payerSecretTag tags the hash of the node key that all payer keys
	// are derived from.
	payerSecretTag = "bolt12/payer-secret"

	// invreqMetadataTag separates the metadata derivation from the payer
	// key derivation under the same secret.
	invreqMetadataTag = "bolt12/invreq-metadata"

	// payerKeyTag separates the payer key derivation from the metadata
	// derivation under the same secret.
	payerKeyTag = "bolt12/payer-key"

	// payOfferParamsTag tags the hash that commits to the parameters of
	// an offer payment.
	payOfferParamsTag = "bolt12/pay-offer-params"
)

// errZeroPayerKey is returned in the negligible case that a derivation gives
// no valid private key.
var errZeroPayerKey = errors.New("derived payer key is not a valid scalar")

// PayerKey is the payer side of an invoice request that is derived from the
// caller's idempotency key. The same key and offer always give the same
// metadata and payer key, so a retry builds a byte-identical request, and the
// node stores neither.
type PayerKey struct {
	// Metadata is the invreq_metadata of the request.
	Metadata []byte

	// PrivKey is the key whose public key is invreq_payer_id.
	PrivKey *btcec.PrivateKey
}

// PayerSecret derives the secret that payer keys come from. It is a tagged
// hash of the node key, so it never leaves the node, and outsiders cannot
// link two requests or guess the metadata of one.
func PayerSecret(nodeKey *btcec.PrivateKey) [32]byte {
	return *chainhash.TaggedHash([]byte(payerSecretTag), nodeKey.Serialize())
}

// DerivePayerKey derives the metadata and the payer key of an invoice request
// from an idempotency key and the offer it pays. The offer hash is part of the
// derivation, so one key used for two offers never shows the same payer id to
// both.
func DerivePayerKey(secret [32]byte, idempotencyKey []byte,
	offerHash [32]byte) (*PayerKey, error) {

	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(invreqMetadataTag))
	mac.Write(offerHash[:])
	mac.Write(idempotencyKey)
	metadata := mac.Sum(nil)

	privKey, err := PayerPrivKeyFromMetadata(secret, metadata)
	if err != nil {
		return nil, err
	}

	return &PayerKey{
		Metadata: metadata,
		PrivKey:  privKey,
	}, nil
}

// PayerPrivKeyFromMetadata derives the payer key from request metadata. An
// invoice mirrors the metadata, so the node can recover the payer key of a
// paid invoice, which proof of payer needs, without storing it.
func PayerPrivKeyFromMetadata(secret [32]byte,
	metadata []byte) (*btcec.PrivateKey, error) {

	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(payerKeyTag))
	mac.Write(metadata)

	var scalar btcec.ModNScalar
	overflow := scalar.SetByteSlice(mac.Sum(nil))
	if overflow || scalar.IsZero() {
		return nil, errZeroPayerKey
	}

	return btcec.PrivKeyFromScalar(&scalar), nil
}

// PayOfferParamsHash commits to the parameters that define an offer payment:
// the offer, the amount, the quantity and the payer note. A known idempotency
// key with another hash is a different request, and the node refuses it.
func PayOfferParamsHash(offerHash [32]byte, amountMsat, quantity uint64,
	payerNote string) [32]byte {

	var num [8]byte
	msg := make([]byte, 0, 32+8+8+len(payerNote))
	msg = append(msg, offerHash[:]...)

	binary.BigEndian.PutUint64(num[:], amountMsat)
	msg = append(msg, num[:]...)

	binary.BigEndian.PutUint64(num[:], quantity)
	msg = append(msg, num[:]...)

	msg = append(msg, payerNote...)

	return *chainhash.TaggedHash([]byte(payOfferParamsTag), msg)
}
