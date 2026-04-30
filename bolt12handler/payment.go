package bolt12handler

import (
	"fmt"
	"time"

	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing"
	"github.com/lightningnetwork/lnd/tlv"
)

const (
	// defaultFeeLimitPercent is the default fee limit as a percentage of
	// the payment amount when no explicit fee limit is provided.
	defaultFeeLimitPercent = 1

	// defaultMaxParts is the upper bound on MPP shards used for BOLT 12
	// payments when the invoice advertises OPT_BASIC_MPP. Mirrors the
	// router's DefaultMaxParts for BOLT 11.
	defaultMaxParts = 16
)

// Bolt12InvoiceToBlindedPathSet converts the blinded payment paths from a BOLT
// 12 invoice into a routing.BlindedPaymentPathSet suitable for the LND router.
// Sciddir introduction nodes are resolved through the supplied resolver; a nil
// resolver is acceptable for pubkey-only invoices and surfaces an error if
// any path uses the sciddir form.
func Bolt12InvoiceToBlindedPathSet(inv *bolt12.Invoice,
	resolver lnwire.IntroNodeResolver) (
	*routing.BlindedPaymentPathSet, error) {

	// UsablePaths pairs each invoice_paths entry with its blinded_payinfo
	// and drops any path the BOLT 12 reader must not use (an even, unknown
	// required feature bit). It also returns empty when invoice_paths or
	// invoice_blindedpay is absent, or when the two lists differ in length,
	// so an empty result is the single "no path we may pay" signal here.
	usable := inv.UsablePaths(bolt12.Bolt12Features)
	if len(usable) == 0 {
		return nil, fmt.Errorf("invoice has no usable blinded paths")
	}

	blindedPayments := make([]*routing.BlindedPayment, len(usable))
	for i, up := range usable {
		payment, err := convertBlindedPath(up.Path, up.PayInfo, resolver)
		if err != nil {
			return nil, fmt.Errorf("path %d: %w", i, err)
		}

		if err := payment.Validate(); err != nil {
			return nil, fmt.Errorf("path %d validation: %w",
				i, err)
		}

		blindedPayments[i] = payment
	}

	return routing.NewBlindedPaymentPathSet(blindedPayments)
}

// convertBlindedPath converts a single lnwire.BlindedPath and its corresponding
// BlindedPayInfo into a routing.BlindedPayment. The resolver is consulted for
// sciddir introduction nodes; pubkey-form intros bypass it.
func convertBlindedPath(path lnwire.BlindedPath,
	info bolt12.BlindedPayInfo,
	resolver lnwire.IntroNodeResolver) (*routing.BlindedPayment, error) {

	sphinxPath, err := path.ToSphinx(resolver)
	if err != nil {
		return nil, fmt.Errorf("convert blinded path: %w", err)
	}

	return &routing.BlindedPayment{
		BlindedPath:         sphinxPath,
		BaseFee:             info.FeeBaseMsat,
		ProportionalFeeRate: info.FeeProportionalMillionths,
		CltvExpiryDelta:     info.CltvExpiryDelta,
		HtlcMinimum:         info.HtlcMinimumMsat,
		HtlcMaximum:         info.HtlcMaximumMsat,
	}, nil
}

// BuildLightningPayment constructs a routing.LightningPayment from a validated
// BOLT 12 invoice and its blinded path set. The invoiceString (lni1...) is
// stored as the PaymentRequest for intent_payload persistence. The offerID
// propagates to PaymentCreationInfo for indexed offer-level queries.
func BuildLightningPayment(inv *bolt12.Invoice,
	pathSet *routing.BlindedPaymentPathSet,
	invoiceString string, offerID []byte, feeLimitMsat int64,
	timeoutSeconds uint64) (*routing.LightningPayment, error) {

	// Extract payment hash from the typed [32]byte field.
	var (
		payHash      lntypes.Hash
		payHashFound bool
	)
	inv.InvoicePaymentHash.WhenSome(
		func(r tlv.RecordT[tlv.TlvType168, [32]byte]) {
			copy(payHash[:], r.Val[:])
			payHashFound = true
		},
	)
	if !payHashFound {
		return nil, fmt.Errorf("invoice missing payment hash")
	}

	// Extract invoice amount.
	amount := lnwire.MilliSatoshi(getInvoiceAmount(inv))
	if amount == 0 {
		return nil, fmt.Errorf("invoice has zero amount")
	}

	// Determine fee limit.
	feeLimit := lnwire.MilliSatoshi(feeLimitMsat)
	if feeLimit == 0 {
		feeLimit = amount * defaultFeeLimitPercent / 100
		if feeLimit == 0 {
			feeLimit = 1
		}
	}

	// Determine timeout.
	timeout := 60 * time.Second
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}

	payment := &routing.LightningPayment{
		Amount:            amount,
		FeeLimit:          feeLimit,
		BlindedPathSet:    pathSet,
		PayAttemptTimeout: timeout,
		MaxParts:          1,
		PaymentRequest:    []byte(invoiceString),
		PaymentAddr:       fn.None[[32]byte](),
		OfferID:           offerID,
	}

	if err := payment.SetPaymentHash(payHash); err != nil {
		return nil, fmt.Errorf("set payment hash: %w", err)
	}

	// Set the target to the blinded path set's target pubkey.
	copy(
		payment.Target[:],
		pathSet.TargetPubKey().SerializeCompressed(),
	)

	// Seed DestFeatures from the blinded path set features (relay
	// capabilities). When the invoice advertises OPT_BASIC_MPP, declare
	// MPP plus its dependency chain (payment_addr, tlv_onion) so the
	// router's splitter and feature.ValidateDeps both pass; BOLT 12
	// supplies these implicitly via blinded-path encrypted data.
	destFeatures := lnwire.EmptyFeatureVector()
	if pathFeatures := pathSet.Features(); pathFeatures != nil {
		destFeatures = pathFeatures.Clone()
	}
	if invoiceAdvertisesMPP(inv) {
		destFeatures.Set(lnwire.MPPOptional)
		destFeatures.Set(lnwire.PaymentAddrOptional)
		destFeatures.Set(lnwire.TLVOnionPayloadOptional)
		payment.MaxParts = defaultMaxParts
	}
	if !destFeatures.IsEmpty() {
		payment.DestFeatures = destFeatures
	}

	return payment, nil
}

// invoiceAdvertisesMPP returns true if the invoice's invoice_features (TLV
// 174) advertises OPT_BASIC_MPP (bit 17). A receiver that does not set this
// bit must be paid as a single shard.
func invoiceAdvertisesMPP(inv *bolt12.Invoice) bool {
	advertises := false
	inv.InvoiceFeatures.WhenSome(
		func(r tlv.RecordT[tlv.TlvType174, lnwire.RawFeatureVector]) {
			advertises = r.Val.IsSet(lnwire.MPPOptional) ||
				r.Val.IsSet(lnwire.MPPRequired)
		},
	)
	return advertises
}
