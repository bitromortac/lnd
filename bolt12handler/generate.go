package bolt12handler

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/bits"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/record"
	"github.com/lightningnetwork/lnd/routing"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/lightningnetwork/lnd/zpay32"
)

// errAmountOverflow is returned when offer_amount times invreq_quantity does
// not fit in a uint64.
var errAmountOverflow = errors.New("invoice amount overflows uint64")

// errCurrencyConversion is returned when the invoice amount needs a
// conversion from offer_currency to msat. The handler has no exchange rate.
var errCurrencyConversion = errors.New("offer_currency amount needs " +
	"conversion to msat")

// FinalCLTVDelta is the final CLTV delta that this node requires for a BOLT 12
// invoice payment. The receiver stores it with the invoice, and the payment
// paths add routing.BlockPadding to it.
const FinalCLTVDelta = zpay32.DefaultAssumedFinalCLTVDelta

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

// GenerateInvoice creates a BOLT 12 invoice in response to a validated invoice
// request. The invoice mirrors the request. If pathBuilder is nil or fails, a
// single-hop blinded path is used as fallback.
func GenerateInvoice(ir *bolt12.InvoiceRequest,
	signer NodeSigner, pathBuilder PaymentPathBuilder,
	offerHash [32]byte,
	pathKey *btcec.PublicKey) (*InvoiceResult, error) {

	var preimage lntypes.Preimage
	if _, err := rand.Read(preimage[:]); err != nil {
		return nil, fmt.Errorf("generate preimage: %w", err)
	}
	paymentHash := preimage.Hash()

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
	invoiceAmount, err := computeInvoiceAmount(ir)
	if err != nil {
		return nil, err
	}
	envData := &InvoiceEnvelopeData{
		Preimage:  [32]byte(preimage),
		CreatedAt: uint64(time.Now().Unix()),
		Amount:    invoiceAmount,
		Quantity:  uint64(ir.InvreqQuantity.ValOpt().UnwrapOr(0)),
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

	var pathResult *PaymentPathResult
	if pathBuilder != nil {
		pathResult, err = pathBuilder.BuildPaymentPaths(
			invoiceAmount, pathID[:], envelopeBytes,
		)
		if err != nil {
			log.Debugf("Multi-hop payment path construction "+
				"failed, falling back to single-hop: %v",
				err)

			pathResult = nil
		}
	}

	// TODO: The single-hop path names our node as the introduction node.
	// For a blinded offer, this reveals the node that the offer hides.
	// Revisit the fallback for privacy when blinded offers are supported.
	if pathResult == nil {
		path, pathErr := buildSingleHopBlindedPath(
			signer.NodePubKey(), pathID[:], envelopeBytes,
		)
		if pathErr != nil {
			return nil, fmt.Errorf("build single-hop path: %w",
				pathErr)
		}

		log.Debugf("Using single-hop blinded payment path")

		pathResult = &PaymentPathResult{
			Paths: []lnwire.BlindedPath{path},
			PayInfos: []bolt12.BlindedPayInfo{{
				FeeBaseMsat:               0,
				FeeProportionalMillionths: 0,
				// Add BlockPadding so that the receiver does not
				// reject the HTLC if blocks are mined while the
				// payment is in flight. With blinded paths, the
				// receiver adds this padding, not the sender.
				CltvExpiryDelta: FinalCLTVDelta +
					routing.BlockPadding,
				HtlcMinimumMsat: 0,
				HtlcMaximumMsat: invoiceAmount,
			}},
		}
	} else {
		log.Debugf("Using multi-hop blinded payment path with "+
			"%d path(s)", len(pathResult.Paths))
	}

	// Choose the identity the invoice is signed under. An offer that
	// publishes offer_paths instead of offer_issuer_id binds
	// invoice_node_id to the blinded node the payer reached, so signing
	// under our identity key would produce an invoice the payer must
	// reject. The payment path's introduction node stays the real node ID
	// in both cases, since the payer has to route to it.
	invoiceNodeID, signInvoice, err := invoiceSigningIdentity(
		ir, signer, pathKey,
	)
	if err != nil {
		return nil, err
	}

	inv := buildInvoiceFromRequest(
		ir, invoiceNodeID, paymentHash, pathResult,
		invoiceAmount,
	)

	signedInv, encoded, err := signAndEncode(inv, signInvoice)
	if err != nil {
		return nil, fmt.Errorf("sign invoice: %w", err)
	}

	return &InvoiceResult{
		Invoice:     signedInv,
		Encoded:     encoded,
		Preimage:    preimage,
		PaymentHash: paymentHash,
		PathID:      pathID,
	}, nil
}

// invoiceSigner produces the Schnorr signature for an invoice over its Merkle
// root. It abstracts which identity signs, so the caller resolves that once
// and the signing path stays unaware of blinding.
type invoiceSigner func(inv *bolt12.Invoice) ([64]byte, error)

// invoiceSigningIdentity resolves the key an invoice must be signed under and
// the invoice_node_id that names it.
//
// The spec gives offer_issuer_id precedence: when the offer published one, the
// payer checks invoice_node_id against it and our identity key is correct.
// With offer_paths and no offer_issuer_id there is no published identity to
// sign under, so the binding falls to the blinded_node_id of the path the
// request arrived on. Refusing to sign when that path key is unknown is
// deliberate. An invoice signed under the wrong identity is rejected by any
// conforming payer, so failing here reports the real fault instead of
// deferring it to a confusing validation error at the payer.
func invoiceSigningIdentity(ir *bolt12.InvoiceRequest, signer NodeSigner,
	pathKey *btcec.PublicKey) (*btcec.PublicKey, invoiceSigner, error) {

	if ir.OfferIssuerID.IsSome() {
		return signer.NodePubKey(), signer.SignInvoice, nil
	}

	if pathKey == nil {
		return nil, nil, fmt.Errorf("offer has no offer_issuer_id " +
			"and the arrival path key is unknown, so " +
			"invoice_node_id cannot be bound to a blinded node")
	}

	blindedNodeID, err := signer.BlindedNodePubKey(pathKey)
	if err != nil {
		return nil, nil, fmt.Errorf("derive blinded node id: %w", err)
	}

	signInvoice := func(inv *bolt12.Invoice) ([64]byte, error) {
		return signer.SignInvoiceBlinded(inv, pathKey)
	}

	return blindedNodeID, signInvoice, nil
}

// buildInvoiceFromRequest constructs a bolt12.Invoice by mirroring the request
// fields and adding invoice-specific fields.
func buildInvoiceFromRequest(ir *bolt12.InvoiceRequest,
	nodePubKey *btcec.PublicKey, paymentHash lntypes.Hash,
	pathResult *PaymentPathResult,
	invoiceAmount uint64) *bolt12.Invoice {

	// The codec mirrors the request, including unknown TLVs in the signed
	// range. The payer's byte-for-byte check requires them, and a
	// field-by-field copy could not reach them.
	inv := bolt12.NewInvoiceFromRequest(ir)

	now := bolt12.TUint64(time.Now().Unix())
	inv.InvoiceCreatedAt = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType164, bolt12.TUint64]{
			Val: now,
		},
	)

	// TODO: Support a custom invoice expiry from the config. Set
	// invoice_relative_expiry here, and store the invoice with the same
	// expiry.
	amt := bolt12.TUint64(invoiceAmount)
	inv.InvoiceAmount = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType170, bolt12.TUint64]{
			Val: amt,
		},
	)

	inv.InvoicePaymentHash = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType168, [32]byte](paymentHash),
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

	// Advertise OPT_BASIC_MPP so payers can split a payment that exceeds
	// the capacity of one channel.
	inv.InvoiceFeatures = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType174, lnwire.RawFeatureVector]{
			Val: *lnwire.NewRawFeatureVector(lnwire.MPPOptional),
		},
	)

	return inv
}

// computeInvoiceAmount returns the amount the invoice must pay. The request's
// invreq_amount takes precedence. Without it, the amount comes from the offer,
// and a currency offer or an overflowing product returns an error.
func computeInvoiceAmount(ir *bolt12.InvoiceRequest) (uint64, error) {
	if ir.InvreqAmount.IsSome() {
		return uint64(ir.InvreqAmount.ValOpt().UnwrapOr(0)), nil
	}

	// The offer_amount unit is offer_currency when that field is set, so
	// it is not a msat value.
	//
	// TODO: Support offers priced in a currency. This needs an exchange
	// rate to convert offer_amount to msat.
	if ir.OfferCurrency.IsSome() {
		return 0, errCurrencyConversion
	}

	// The reader's overflow check runs only when invreq_amount is set, so
	// a payer could otherwise pick a quantity that wraps the product.
	return expectedOfferAmount(
		uint64(ir.OfferAmount.ValOpt().UnwrapOr(0)),
		uint64(ir.InvreqQuantity.ValOpt().UnwrapOr(0)),
	)
}

// expectedOfferAmount returns offer_amount times the quantity, the expected
// amount of an offer priced in the chain's currency. A quantity of zero means
// that the request has none, which counts as one item.
func expectedOfferAmount(offerAmount, quantity uint64) (uint64, error) {
	if quantity == 0 {
		quantity = 1
	}

	hi, amount := bits.Mul64(offerAmount, quantity)
	if hi != 0 {
		return 0, errAmountOverflow
	}

	return amount, nil
}

// buildSingleHopBlindedPath creates a single-hop blinded payment path for the
// direct-peer case. The introduction node is the receiver itself, and the
// single hop's encrypted data carries the path_id for invoice lookup.
func buildSingleHopBlindedPath(nodePubKey *btcec.PublicKey,
	pathID []byte,
	invoiceEnvelope []byte) (lnwire.BlindedPath, error) {

	sessionKey, err := btcec.NewPrivateKey()
	if err != nil {
		return lnwire.BlindedPath{}, fmt.Errorf("generate session "+
			"key: %w", err)
	}

	// The path_id lets the receiver match the incoming HTLC, and the
	// optional envelope lets it reconstruct the invoice.
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

// signAndEncode attaches the signature to inv in place and returns the bech32
// string of the signed invoice.
func signAndEncode(inv *bolt12.Invoice,
	signInvoice invoiceSigner) (*bolt12.Invoice, string, error) {

	// The signer runs the writer validation first, so a malformed invoice
	// fails before a key signs it.
	sig, err := signInvoice(inv)
	if err != nil {
		return nil, "", fmt.Errorf("sign: %w", err)
	}

	inv.Signature = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType240, [64]byte](sig),
	)

	encoded, err := bolt12.EncodeInvoiceString(inv)
	if err != nil {
		return nil, "", err
	}

	return inv, encoded, nil
}
