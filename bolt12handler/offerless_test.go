package bolt12handler

import (
	"bytes"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// offerlessSetup is a payer that published an invoice request without an
// offer, and the payee that answers it.
type offerlessSetup struct {
	payerSigner *privKeySigner
	payeeSigner *privKeySigner
	payer       *PayerKey
	path        lnwire.BlindedPath
	request     *bolt12.InvoiceRequest
}

// newOfferlessSetup builds a signed request without an offer, with one real
// blinded path to the payer as its invreq_paths.
func newOfferlessSetup(t *testing.T) *offerlessSetup {
	t.Helper()

	payerNode := newTestPrivKey(t)
	payerSigner := newPrivKeySigner(payerNode)

	path, err := buildSingleHopBlindedPath(
		payerNode.PubKey(), bytes.Repeat([]byte{1}, 32), nil,
	)
	require.NoError(t, err)

	payer, err := DerivePayerKey(
		PayerSecret(payerNode), []byte("refund-1"), [32]byte{},
	)
	require.NoError(t, err)

	req, err := BuildOfferlessInvoiceRequest(payer, OfferlessRequestParams{
		Description: "refund",
		AmountMsat:  25000,
		Chain:       testChainHash(),
		Paths:       []lnwire.BlindedPath{path},
	})
	require.NoError(t, err)

	return &offerlessSetup{
		payerSigner: payerSigner,
		payeeSigner: newPrivKeySigner(newTestPrivKey(t)),
		payer:       payer,
		path:        path,
		request:     req,
	}
}

// TestBuildOfferlessInvoiceRequest verifies that the request passes the
// offer-less reader as an lnr1 string, and that the same key builds the same
// bytes.
func TestBuildOfferlessInvoiceRequest(t *testing.T) {
	t.Parallel()

	s := newOfferlessSetup(t)

	lnr, err := bolt12.EncodeInvoiceRequestString(s.request)
	require.NoError(t, err)

	scanned, err := bolt12.DecodeInvoiceRequestString(lnr, testChainHash())
	require.NoError(t, err)
	require.False(t, scanned.OfferIssuerID.IsSome())
	require.True(t, scanned.InvreqPaths.IsSome())

	again, err := BuildOfferlessInvoiceRequest(
		s.payer, OfferlessRequestParams{
			Description: "refund",
			AmountMsat:  25000,
			Chain:       testChainHash(),
			Paths:       []lnwire.BlindedPath{s.path},
		},
	)
	require.NoError(t, err)

	first, err := s.request.EncodeSigned()
	require.NoError(t, err)
	second, err := again.EncodeSigned()
	require.NoError(t, err)
	require.Equal(t, first, second)

	_, err = BuildOfferlessInvoiceRequest(
		s.payer, OfferlessRequestParams{
			Description: "refund", AmountMsat: 25000,
			Chain: testChainHash(),
		},
	)
	require.ErrorIs(t, err, ErrNoInvreqPaths)
}

// TestOfferlessInvoiceRoundTrip walks both sides: the payee answers the
// request with an invoice under its node key, and the payer accepts it only
// when it arrived on the request's path and comes from the expected node.
func TestOfferlessInvoiceRoundTrip(t *testing.T) {
	t.Parallel()

	s := newOfferlessSetup(t)

	result, err := GenerateOfferlessInvoice(s.request, s.payeeSigner, nil)
	require.NoError(t, err)

	inv := result.Invoice
	require.True(t, inv.InvoiceNodeID.ValOpt().UnwrapOr(nil).IsEqual(
		s.payeeSigner.NodePubKey(),
	))
	require.Equal(
		t, bolt12.TUint64(25000), inv.InvoiceAmount.ValOpt().UnwrapOr(0),
	)

	now := time.Now()
	validate := func(pathKey, expected *btcec.PublicKey) error {
		return ValidateOfferlessInvoice(
			inv, s.request, s.payerSigner, pathKey, expected,
			testChainHash(), now,
		)
	}

	// Arrived on the request's path, from the agreed node.
	require.NoError(t, validate(
		s.path.BlindingPoint, s.payeeSigner.NodePubKey(),
	))

	// No agreed node: the codec accepts it, and the caller asks the user.
	require.NoError(t, validate(s.path.BlindingPoint, nil))

	// From a node other than the agreed one.
	err = validate(s.path.BlindingPoint, newTestPrivKey(t).PubKey())
	require.ErrorIs(t, err, bolt12.ErrUnexpectedInvoiceNodeID)

	// Arrived on another path.
	err = validate(newTestPrivKey(t).PubKey(), nil)
	require.ErrorIs(t, err, ErrWrongInvreqPath)
}

// TestOfferlessRefusesOffers verifies that the offer-less operations refuse a
// request or an invoice that answers an offer.
func TestOfferlessRefusesOffers(t *testing.T) {
	t.Parallel()

	s := newOfferlessSetup(t)
	key := testKey(t)

	withOffer := *s.request
	withOffer.OfferIssuerID = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType22](key.PubKey()),
	)
	_, err := GenerateOfferlessInvoice(&withOffer, s.payeeSigner, nil)
	require.ErrorIs(t, err, ErrNotOfferless)

	result, err := GenerateOfferlessInvoice(s.request, s.payeeSigner, nil)
	require.NoError(t, err)

	inv := *result.Invoice
	inv.OfferIssuerID = withOffer.OfferIssuerID
	err = ValidateOfferlessInvoice(
		&inv, s.request, s.payerSigner, s.path.BlindingPoint, nil,
		testChainHash(), time.Now(),
	)
	require.ErrorIs(t, err, ErrNotOfferless)
}
