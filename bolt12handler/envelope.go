package bolt12handler

import (
	"bytes"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/tlv"
)

const (
	// envelopeSignatureTag is the BIP-340 tagged hash domain separator for
	// envelope signatures. This prevents cross-protocol signature confusion
	// with BOLT 12 invoice Merkle signatures (which use "lightning..." tags),
	// channel announcements, and other Schnorr uses in the system.
	envelopeSignatureTag = "bolt12/envelope"

	// envelopeTLVTypePreimage is the TLV type for the 32-byte payment
	// preimage inside InvoiceEnvelopeData.
	envelopeTLVTypePreimage tlv.Type = 0

	// envelopeTLVTypePayerID is the TLV type for the 33-byte compressed
	// public key of the payer (invreq_payer_id).
	envelopeTLVTypePayerID tlv.Type = 2

	// envelopeTLVTypeCreatedAt is the TLV type for the invoice creation
	// timestamp in seconds since epoch.
	envelopeTLVTypeCreatedAt tlv.Type = 4

	// envelopeTLVTypeAmount is the TLV type for the invoice amount in
	// millisatoshis.
	envelopeTLVTypeAmount tlv.Type = 6
)

// InvoiceEnvelopeData contains the fields needed to reconstruct a BOLT 12
// invoice at HTLC settlement time. This is a node-internal structure — the same
// node writes and reads it — so the BOLT even/odd interop convention does not
// apply.
type InvoiceEnvelopeData struct {
	// Preimage is the random 32-byte payment preimage.
	Preimage [32]byte

	// PayerID is the 33-byte compressed public key from invreq_payer_id.
	PayerID [33]byte

	// CreatedAt is the invoice creation timestamp (seconds since epoch).
	CreatedAt uint64

	// Amount is the invoice amount in millisatoshis.
	Amount uint64
}

// SignedInvoiceEnvelope wraps the TLV-encoded InvoiceEnvelopeData with a
// Schnorr signature and the offer ID hash. The signature covers
// tagged_hash("bolt12/envelope", offerIDHash || tlvData).
type SignedInvoiceEnvelope struct {
	// Signature is the 64-byte BIP-340 Schnorr signature over the tagged
	// hash of the envelope contents.
	Signature [64]byte

	// OfferIDHash is the SHA-256 of the offer's TLV encoding, identifying
	// which offer this envelope belongs to.
	OfferIDHash [32]byte

	// TLVData is the TLV-encoded InvoiceEnvelopeData.
	TLVData []byte
}

// EncodeEnvelopeData serializes an InvoiceEnvelopeData into canonical TLV
// bytes. The output is deterministic: fields are encoded in ascending type
// order.
func EncodeEnvelopeData(data *InvoiceEnvelopeData) ([]byte, error) {
	preimage := data.Preimage[:]
	payerID := data.PayerID[:]

	tlvStream, err := tlv.NewStream(
		tlv.MakePrimitiveRecord(
			envelopeTLVTypePreimage, &preimage,
		),
		tlv.MakePrimitiveRecord(
			envelopeTLVTypePayerID, &payerID,
		),
		tlv.MakePrimitiveRecord(
			envelopeTLVTypeCreatedAt, &data.CreatedAt,
		),
		tlv.MakePrimitiveRecord(
			envelopeTLVTypeAmount, &data.Amount,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create tlv stream: %w", err)
	}

	var buf bytes.Buffer
	if err := tlvStream.Encode(&buf); err != nil {
		return nil, fmt.Errorf("encode tlv: %w", err)
	}

	return buf.Bytes(), nil
}

// DecodeEnvelopeData deserializes TLV bytes into an InvoiceEnvelopeData.
// Unknown TLV types are silently ignored for forward compatibility.
func DecodeEnvelopeData(b []byte) (*InvoiceEnvelopeData, error) {
	var (
		data     InvoiceEnvelopeData
		preimage []byte
		payerID  []byte
	)

	tlvStream, err := tlv.NewStream(
		tlv.MakePrimitiveRecord(
			envelopeTLVTypePreimage, &preimage,
		),
		tlv.MakePrimitiveRecord(
			envelopeTLVTypePayerID, &payerID,
		),
		tlv.MakePrimitiveRecord(
			envelopeTLVTypeCreatedAt, &data.CreatedAt,
		),
		tlv.MakePrimitiveRecord(
			envelopeTLVTypeAmount, &data.Amount,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create tlv stream: %w", err)
	}

	r := bytes.NewReader(b)
	parsedTypes, err := tlvStream.DecodeWithParsedTypes(r)
	if err != nil {
		return nil, fmt.Errorf("decode tlv: %w", err)
	}

	// Verify all required fields were present.
	if _, ok := parsedTypes[envelopeTLVTypePreimage]; !ok {
		return nil, fmt.Errorf("missing preimage (type %d)",
			envelopeTLVTypePreimage)
	}

	if _, ok := parsedTypes[envelopeTLVTypePayerID]; !ok {
		return nil, fmt.Errorf("missing payer_id (type %d)",
			envelopeTLVTypePayerID)
	}

	if _, ok := parsedTypes[envelopeTLVTypeCreatedAt]; !ok {
		return nil, fmt.Errorf("missing created_at (type %d)",
			envelopeTLVTypeCreatedAt)
	}

	if _, ok := parsedTypes[envelopeTLVTypeAmount]; !ok {
		return nil, fmt.Errorf("missing amount (type %d)",
			envelopeTLVTypeAmount)
	}

	if len(preimage) != 32 {
		return nil, fmt.Errorf("invalid preimage length: %d",
			len(preimage))
	}
	copy(data.Preimage[:], preimage)

	if len(payerID) != 33 {
		return nil, fmt.Errorf("invalid payer_id length: %d",
			len(payerID))
	}
	copy(data.PayerID[:], payerID)

	return &data, nil
}

// envelopeDigest computes the tagged hash digest for signing or verification:
// tagged_hash("bolt12/envelope", offerIDHash || tlvData).
func envelopeDigest(offerIDHash [32]byte, tlvData []byte) [32]byte {
	msg := make([]byte, 32+len(tlvData))
	copy(msg[:32], offerIDHash[:])
	copy(msg[32:], tlvData)

	return *chainhash.TaggedHash(
		[]byte(envelopeSignatureTag), msg,
	)
}

// SignEnvelope signs an InvoiceEnvelopeData with the provided private key and
// returns a SignedInvoiceEnvelope. The signature covers
// tagged_hash("bolt12/envelope", offerIDHash || encodedData).
func SignEnvelope(privKey *btcec.PrivateKey, offerIDHash [32]byte,
	data *InvoiceEnvelopeData) (*SignedInvoiceEnvelope, error) {

	tlvData, err := EncodeEnvelopeData(data)
	if err != nil {
		return nil, fmt.Errorf("encode envelope data: %w", err)
	}

	digest := envelopeDigest(offerIDHash, tlvData)

	sig, err := schnorr.Sign(privKey, digest[:])
	if err != nil {
		return nil, fmt.Errorf("sign envelope: %w", err)
	}

	var sigBytes [64]byte
	copy(sigBytes[:], sig.Serialize())

	return &SignedInvoiceEnvelope{
		Signature:   sigBytes,
		OfferIDHash: offerIDHash,
		TLVData:     tlvData,
	}, nil
}

// VerifyEnvelope verifies the signature on a SignedInvoiceEnvelope and returns
// the decoded InvoiceEnvelopeData. If the signature is invalid or the data
// cannot be decoded, an error is returned.
func VerifyEnvelope(pubKey *btcec.PublicKey,
	signed *SignedInvoiceEnvelope) (*InvoiceEnvelopeData, error) {

	digest := envelopeDigest(signed.OfferIDHash, signed.TLVData)

	parsedSig, err := schnorr.ParseSignature(signed.Signature[:])
	if err != nil {
		return nil, fmt.Errorf("parse signature: %w", err)
	}

	if !parsedSig.Verify(digest[:], pubKey) {
		return nil, fmt.Errorf("envelope signature verification failed")
	}

	data, err := DecodeEnvelopeData(signed.TLVData)
	if err != nil {
		return nil, fmt.Errorf("decode envelope data: %w", err)
	}

	return data, nil
}

// EncodeSignedEnvelope serializes a SignedInvoiceEnvelope into bytes:
// [64-byte signature][32-byte offerIDHash][variable TLVData].
func EncodeSignedEnvelope(env *SignedInvoiceEnvelope) []byte {
	buf := make([]byte, 64+32+len(env.TLVData))
	copy(buf[:64], env.Signature[:])
	copy(buf[64:96], env.OfferIDHash[:])
	copy(buf[96:], env.TLVData)

	return buf
}

// DecodeSignedEnvelope deserializes bytes into a SignedInvoiceEnvelope. The
// minimum length is 96 bytes (64-byte signature + 32-byte offerIDHash).
func DecodeSignedEnvelope(b []byte) (*SignedInvoiceEnvelope, error) {
	if len(b) < 96 {
		return nil, fmt.Errorf("envelope too short: %d bytes, "+
			"minimum 96", len(b))
	}

	var env SignedInvoiceEnvelope
	copy(env.Signature[:], b[:64])
	copy(env.OfferIDHash[:], b[64:96])
	env.TLVData = make([]byte, len(b)-96)
	copy(env.TLVData, b[96:])

	return &env, nil
}
