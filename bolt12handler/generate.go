package bolt12handler

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/record"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/lightningnetwork/lnd/zpay32"
)

// PaymentPathResult contains the blinded payment paths and corresponding pay
// info for a BOLT 12 invoice.
type PaymentPathResult struct {
	// Paths contains the blinded payment paths for the invoice.
	Paths []lnwire.BlindedPath

	// PayInfos contains the fee and CLTV policy for each path.
	PayInfos []bolt12.BlindedPayInfo
}

// PaymentPathBuilder constructs blinded payment paths for a BOLT 12 invoice.
// The builder receives the invoice amount, a path_id, and an optional signed
// envelope to embed in the final hop's encrypted data.
type PaymentPathBuilder interface {
	// BuildPaymentPaths returns blinded payment paths suitable for
	// embedding in a BOLT 12 invoice.
	BuildPaymentPaths(amountMsat uint64, pathID []byte,
		invoiceEnvelope []byte) (*PaymentPathResult, error)
}

// InvoiceResult contains the output of invoice generation.
type InvoiceResult struct {
	// Invoice is the generated BOLT 12 invoice.
	Invoice *bolt12.Invoice

	// Encoded is the bech32-encoded invoice string (lni1...).
	Encoded string

	// Preimage is the 32-byte payment preimage.
	Preimage lntypes.Preimage

	// PaymentHash is the SHA256 of the preimage.
	PaymentHash lntypes.Hash

	// PathID is the 32-byte path identifier embedded in the blinded path.
	// Used as payment_addr for invoice lookup.
	PathID [32]byte
}

// GenerateInvoice builds a signed BOLT 12 invoice for a validated invoice
// request. It returns the invoice with its preimage, payment hash, and path
// ID. A nil pathBuilder yields a single-hop blinded path.
func GenerateInvoice(ir *bolt12.InvoiceRequest,
	signer NodeSigner, pathBuilder PaymentPathBuilder,
	offerHash [32]byte) (*InvoiceResult, error) {

	// Generate a random preimage and compute the payment hash.
	var preimage lntypes.Preimage
	if _, err := rand.Read(preimage[:]); err != nil {
		return nil, fmt.Errorf("generate preimage: %w", err)
	}
	paymentHash := sha256.Sum256(preimage[:])

	// Generate a random path_id for the blinded path.
	var pathID [32]byte
	if _, err := rand.Read(pathID[:]); err != nil {
		return nil, fmt.Errorf("generate path_id: %w", err)
	}

	// Extract payer ID from the invoice request as the serialised
	// compressed point, for downstream envelope builders that take []byte.
	var payerIDBytes []byte
	ir.InvreqPayerID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType88, *btcec.PublicKey]) {
			payerIDBytes = r.Val.SerializeCompressed()
		},
	)

	// Build the signed envelope for stateless BOLT 12 settlement.
	invoiceAmount := computeInvoiceAmount(ir)
	envData := &InvoiceEnvelopeData{
		Preimage:  [32]byte(preimage),
		CreatedAt: uint64(time.Now().Unix()),
		Amount:    invoiceAmount,
		Quantity:  getUint64Field(ir.InvreqQuantity),
	}
	if len(payerIDBytes) == 33 {
		copy(envData.PayerID[:], payerIDBytes)
	}

	envTLVData, err := EncodeEnvelopeData(envData)
	if err != nil {
		return nil, fmt.Errorf("encode envelope data: %w", err)
	}

	envSig, err := signer.SignEnvelopeData(offerHash, envTLVData)
	if err != nil {
		return nil, fmt.Errorf("sign envelope: %w", err)
	}

	envelopeBytes := EncodeSignedEnvelope(&SignedInvoiceEnvelope{
		Signature: envSig,
		OfferHash: offerHash,
		TLVData:   envTLVData,
	})

	// Build blinded payment paths for the invoice.
	var pathResult *PaymentPathResult
	if pathBuilder != nil {
		pathResult, err = pathBuilder.BuildPaymentPaths(
			invoiceAmount, pathID[:], envelopeBytes,
		)
		if err != nil {
			log.Debugf("Multi-hop payment path construction "+
				"failed, falling back to single-hop: %v",
				err)
		}
	}

	if pathResult == nil {
		// Fall back to single-hop blinded path.
		path, pathErr := buildSingleHopBlindedPath(
			signer.NodePubKey(), pathID[:], envelopeBytes,
		)
		if pathErr != nil {
			return nil, fmt.Errorf("build single-hop path: %w",
				pathErr)
		}

		log.Infof("Using single-hop blinded payment path")

		pathResult = &PaymentPathResult{
			Paths: []lnwire.BlindedPath{path},
			PayInfos: []bolt12.BlindedPayInfo{{
				FeeBaseMsat:               0,
				FeeProportionalMillionths: 0,
				CltvExpiryDelta: zpay32.
					DefaultAssumedFinalCLTVDelta,
				HtlcMinimumMsat: 0,
				HtlcMaximumMsat: invoiceAmount,
			}},
		}
	} else {
		log.Infof("Using multi-hop blinded payment path with "+
			"%d path(s)", len(pathResult.Paths))
	}

	// Build the invoice by copying all non-signature raw TLV fields from
	// the request (types 0-159) for byte-for-byte mirroring, then appending
	// the invoice-specific fields (types >= 160).
	inv := buildInvoiceFromRequest(
		ir, signer.NodePubKey(), paymentHash[:], pathResult,
		invoiceAmount,
	)

	// Sign the invoice over its Merkle root and encode the signed form.
	signedInv, encoded, err := signAndEncode(inv, signer)
	if err != nil {
		return nil, fmt.Errorf("sign invoice: %w", err)
	}

	return &InvoiceResult{
		Invoice:     signedInv,
		Encoded:     encoded,
		Preimage:    preimage,
		PaymentHash: lntypes.Hash(paymentHash),
		PathID:      pathID,
	}, nil
}

// buildInvoiceFromRequest constructs a bolt12.Invoice by mirroring the request
// fields and adding invoice-specific fields.
func buildInvoiceFromRequest(ir *bolt12.InvoiceRequest,
	nodePubKey *btcec.PublicKey, paymentHash []byte,
	pathResult *PaymentPathResult,
	invoiceAmount uint64) *bolt12.Invoice {

	// The codec mirrors the request, including any unknown TLVs in the
	// signed range, which the payer's byte-for-byte check requires and a
	// field-by-field copy here could not reach.
	inv := bolt12.NewInvoiceFromRequest(ir)

	now := bolt12.TUint64(time.Now().Unix())
	inv.InvoiceCreatedAt = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType164, bolt12.TUint64]{
			Val: now,
		},
	)

	amt := bolt12.TUint64(invoiceAmount)
	inv.InvoiceAmount = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType170, bolt12.TUint64]{
			Val: amt,
		},
	)

	// The codec stores the payment hash as a fixed-width [32]byte.
	var paymentHashArr [32]byte
	copy(paymentHashArr[:], paymentHash)
	inv.InvoicePaymentHash = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType168, [32]byte](
			paymentHashArr,
		),
	)

	inv.InvoiceNodeID = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType176](nodePubKey),
	)

	inv.InvoicePaths = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType160, lnwire.BlindedPaths]{
			Val: lnwire.BlindedPaths{
				Paths: pathResult.Paths,
			},
		},
	)

	// One blinded pay info entry per payment path.
	inv.InvoiceBlindedPay = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType162, bolt12.BlindedPayInfos]{
			Val: bolt12.BlindedPayInfos{
				Infos: pathResult.PayInfos,
			},
		},
	)

	// Advertise OPT_BASIC_MPP so senders (LND, CLN xpay >= v25.11,
	// Eclair) multi-part split payments that exceed single-channel
	// capacity.
	inv.InvoiceFeatures = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType174, lnwire.RawFeatureVector]{
			Val: *lnwire.NewRawFeatureVector(lnwire.MPPOptional),
		},
	)

	return inv
}

// computeInvoiceAmount determines the invoice amount from the request. If
// invreq_amount is set, use it. Otherwise multiply offer_amount by
// invreq_quantity (or just offer_amount if no quantity).
func computeInvoiceAmount(ir *bolt12.InvoiceRequest) uint64 {
	if hasOptField(ir.InvreqAmount) {
		return getUint64Field(ir.InvreqAmount)
	}

	amount := getUint64Field(ir.OfferAmount)
	if hasOptField(ir.InvreqQuantity) {
		amount *= getUint64Field(ir.InvreqQuantity)
	}

	return amount
}

// buildSingleHopBlindedPath creates a single-hop blinded payment path for the
// direct-peer case using the sphinx library's blinding primitives. The
// introduction node is the receiver itself, and the single hop's encrypted
// data contains the path_id for invoice lookup.
func buildSingleHopBlindedPath(nodePubKey *btcec.PublicKey,
	pathID []byte,
	invoiceEnvelope []byte) (lnwire.BlindedPath, error) {

	sessionKey, err := btcec.NewPrivateKey()
	if err != nil {
		return lnwire.BlindedPath{}, fmt.Errorf("generate session "+
			"key: %w", err)
	}

	// Encode BlindedRouteData with the path_id and optional envelope so the
	// receiver can match the incoming HTLC and reconstruct the invoice.
	routeData := record.NewFinalHopBlindedRouteData(
		nil, pathID, invoiceEnvelope,
	)

	plainText, err := record.EncodeBlindedRouteData(routeData)
	if err != nil {
		return lnwire.BlindedPath{}, fmt.Errorf("encode route data: %w",
			err)
	}

	hops := []*sphinx.HopInfo{
		{
			NodePub:   nodePubKey,
			PlainText: plainText,
		},
	}

	blindedPath, err := sphinx.BuildBlindedPath(sessionKey, hops)
	if err != nil {
		return lnwire.BlindedPath{}, fmt.Errorf("build blinded "+
			"path: %w", err)
	}

	// Convert sphinx types to bolt12 types.
	path := blindedPath.Path

	bolt12Hops := make([]lnwire.BlindedHop, len(path.BlindedHops))
	for i, hop := range path.BlindedHops {
		bolt12Hops[i] = lnwire.BlindedHop{
			BlindedNodeID: hop.BlindedNodePub,
			EncryptedData: hop.CipherText,
		}
	}

	introNode, err := lnwire.NewPubkeyIntro(nodePubKey)
	if err != nil {
		return lnwire.BlindedPath{}, err
	}

	return lnwire.BlindedPath{
		IntroductionNode: introNode,
		BlindingPoint:    path.BlindingPoint,
		Hops:             bolt12Hops,
	}, nil
}

// signAndEncode validates the invoice against the writer requirements, signs
// it, then encodes it with the signature to produce the final bech32 string.
// Returns the signed invoice and its encoded form.
func signAndEncode(inv *bolt12.Invoice, signer NodeSigner) (*bolt12.Invoice,
	string, error) {

	// Sign the invoice via the node signer. The signer runs the writer
	// validation first, so a malformed invoice fails before a key signs
	// it.
	sig, err := signer.SignInvoice(inv)
	if err != nil {
		return nil, "", fmt.Errorf("sign: %w", err)
	}

	inv.Signature = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType240, [64]byte](sig),
	)

	// Encode the signed invoice. The signature TLV is now part of the
	// message, but verifiers exclude type 240 from the Merkle tree, so
	// the signature stays valid against the root just signed.
	encoded, err := bolt12.EncodeInvoiceString(inv)
	if err != nil {
		return nil, "", err
	}

	return inv, encoded, nil
}
