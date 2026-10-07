package bolt12handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chainhash/v2"
)

const (
	// payerSecretTag tags the hash of the node secret that all payer keys
	// are derived from.
	payerSecretTag = "bolt12/payer-secret"

	// payerSecretPointTag tags the hashes that the payer secret point is
	// derived from.
	payerSecretPointTag = "bolt12/payer-secret-point"

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

// payerSecretPoint is a fixed point with no known private key. Its ECDH with
// the node key gives the payer secret.
var payerSecretPoint = derivePayerSecretPoint()

// derivePayerSecretPoint returns the first point whose x coordinate is the
// tagged hash of a counter. Nobody knows the private key of a point that a
// hash selects.
func derivePayerSecretPoint() *btcec.PublicKey {
	var counter [4]byte
	for i := uint32(0); ; i++ {
		binary.BigEndian.PutUint32(counter[:], i)
		x := chainhash.TaggedHash(
			[]byte(payerSecretPointTag), counter[:],
		)

		point, err := schnorr.ParsePubKey(x[:])
		if err == nil {
			return point
		}
	}
}

// payerSecret derives the secret that payer keys come from. It is a tagged
// hash of the ECDH of the node key and a fixed point that has no known private
// key. So only the node can compute it, and outsiders cannot link two requests
// or guess the metadata of one. ECDH also works with a remote signer, which
// never shows the node key.
func payerSecret(signer Signer) ([32]byte, error) {
	shared, err := signer.ECDH(payerSecretPoint)
	if err != nil {
		return [32]byte{}, fmt.Errorf("payer secret ECDH: %w", err)
	}

	return *chainhash.TaggedHash([]byte(payerSecretTag), shared[:]), nil
}

// DerivePayer derives the payer of an invoice request from an idempotency key
// and the offer it pays. The metadata is an HMAC of both under the payer
// secret, and the payer key is the node key with a tweak that the metadata
// gives, so the signer holds the key. The same key and offer always give the
// same payer, so a retry builds a byte-identical request, and the node stores
// nothing. The offer hash is part of the derivation, so one key used for two
// offers never shows the same payer id to both.
func DerivePayer(signer Signer, idempotencyKey []byte,
	offerHash [32]byte) (*Payer, error) {

	secret, err := payerSecret(signer)
	if err != nil {
		return nil, err
	}

	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(invreqMetadataTag))
	mac.Write(offerHash[:])
	mac.Write(idempotencyKey)
	metadata := mac.Sum(nil)

	return &Payer{
		Signer:   signer,
		Key:      payerKeyFromMetadata(secret, metadata),
		Metadata: metadata,
	}, nil
}

// PayerFromMetadata recovers the payer from request metadata. An invoice
// mirrors the metadata, so the node can recover the payer key of a paid
// invoice, which proof of payer needs, without storing it.
func PayerFromMetadata(signer Signer, metadata []byte) (*Payer, error) {
	secret, err := payerSecret(signer)
	if err != nil {
		return nil, err
	}

	return &Payer{
		Signer:   signer,
		Key:      payerKeyFromMetadata(secret, metadata),
		Metadata: metadata,
	}, nil
}

// payerKeyFromMetadata returns the payer key of the request with metadata: the
// node key with an additive tweak that is an HMAC of the metadata under the
// payer secret.
func payerKeyFromMetadata(secret [32]byte, metadata []byte) KeyRef {
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(payerKeyTag))
	mac.Write(metadata)

	return KeyRef{AddTweak: mac.Sum(nil)}
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
