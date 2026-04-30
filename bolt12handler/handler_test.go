package bolt12handler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/invoices"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/offers"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// mockOfferStore implements offers.Store for testing.
type mockOfferStore struct {
	offers map[[32]byte]*offers.Offer
}

func newMockOfferStore() *mockOfferStore {
	return &mockOfferStore{
		offers: make(map[[32]byte]*offers.Offer),
	}
}

func (m *mockOfferStore) InsertOffer(_ context.Context, offer *offers.Offer) (
	int64, error) {

	m.offers[offer.OfferID] = offer
	offer.ID = int64(len(m.offers))

	return offer.ID, nil
}

func (m *mockOfferStore) GetOffer(_ context.Context, id int64) (*offers.Offer,
	error) {

	for _, o := range m.offers {
		if o.ID == id {
			return o, nil
		}
	}

	return nil, fmt.Errorf("offer %d not found", id)
}

func (m *mockOfferStore) GetOfferByOfferID(_ context.Context,
	offerID [32]byte) (*offers.Offer, error) {

	if o, ok := m.offers[offerID]; ok {
		return o, nil
	}

	return nil, fmt.Errorf("offer not found")
}

func (m *mockOfferStore) ListOffers(_ context.Context, _ bool) ([]*offers.Offer,
	error) {

	var result []*offers.Offer
	for _, o := range m.offers {
		result = append(result, o)
	}

	return result, nil
}

func (m *mockOfferStore) DisableOffer(_ context.Context, _ int64) error {

	return nil
}

// mockNotifier captures BOLT 12 invoice notifications during the handler flow.
type mockNotifier struct {
	invoices []*invoices.Invoice
	hashes   []lntypes.Hash
}

func (m *mockNotifier) NotifyNewBolt12Invoice(hash lntypes.Hash,
	invoice *invoices.Invoice) {

	m.invoices = append(m.invoices, invoice)
	m.hashes = append(m.hashes, hash)
}

// mockReplier captures reply invocations.
type mockReplier struct {
	replies [][]byte
}

func (m *mockReplier) SendInvoiceReply(_ context.Context, invoiceBytes []byte,
	_ *sphinx.BlindedPath) error {

	m.replies = append(m.replies, invoiceBytes)

	return nil
}

// addOffer creates and stores a bolt12 offer in the mock store, returning the
// store offer and its SHA256 offer_id.
func addOffer(t *testing.T, store *mockOfferStore, nodeKey *btcec.PrivateKey,
	amountMsat uint64) *offers.Offer {

	t.Helper()

	offer := testOffer(t, nodeKey, amountMsat)

	// Build the bolt12 offer to compute the offer_id.
	b12Offer := &bolt12.Offer{}
	issuerPub, err := btcec.ParsePubKey(offer.IssuerNodeID[:])
	require.NoError(t, err)
	b12Offer.OfferIssuerID = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType22, *btcec.PublicKey]{
			Val: issuerPub,
		},
	)
	b12Offer.OfferDescription = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType10, tlv.Blob]{
			Val: []byte(offer.Description),
		},
	)
	if offer.HasAmount {
		amt := bolt12.TUint64(offer.AmountMsat)
		b12Offer.OfferAmount = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType8, bolt12.TUint64]{
				Val: amt,
			},
		)
	}

	tlvBytes, err := b12Offer.Encode()
	require.NoError(t, err)

	offer.OfferID = sha256.Sum256(tlvBytes)

	_, err = store.InsertOffer(context.Background(), offer)
	require.NoError(t, err)

	return offer
}

// buildSignedInvreqBytes constructs a signed invoice request and returns the
// raw TLV bytes.
func buildSignedInvreqBytes(t *testing.T, offer *offers.Offer,
	payerKey *btcec.PrivateKey, invreqAmount uint64) []byte {

	t.Helper()

	ir := testInvoiceRequest(t, offer, payerKey, invreqAmount)

	// Encode, decode (to populate rawTLVs), then sign.
	tlvBytes, err := ir.Encode()
	require.NoError(t, err)

	ir, err = bolt12.DecodeInvoiceRequest(tlvBytes)
	require.NoError(t, err)

	sig, err := bolt12.SignInvoiceRequest(ir, payerKey)
	require.NoError(t, err)

	ir.Signature = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType240, [64]byte]{
			Val: sig,
		},
	)

	// Re-encode with signature.
	finalBytes, err := ir.Encode()
	require.NoError(t, err)

	return finalBytes
}

// TestHandleInvoiceRequest_FullFlow exercises the complete handler pipeline:
// decode → validate → offer lookup → invoice generation → registration → reply.
func TestHandleInvoiceRequest_FullFlow(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	store := newMockOfferStore()
	notifier := &mockNotifier{}
	replier := &mockReplier{}

	handler := NewHandler(
		store, notifier, replier, NewPrivKeySigner(nodeKey),
		nil, testChainHash(),
	)

	// Create and store an offer.
	offer := addOffer(t, store, nodeKey, 10000)

	// Build a signed invoice request.
	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	invreqBytes := buildSignedInvreqBytes(
		t, offer, payerKey, 10000,
	)

	// Create a dummy reply path.
	replyPath := &sphinx.BlindedPath{
		IntroductionPoint: nodeKey.PubKey(),
	}

	// Handle the request.
	ctx := context.Background()
	err := handler.HandleInvoiceRequest(ctx, invreqBytes, replyPath)
	require.NoError(t, err)

	// Verify invoice notification was sent (no DB write).
	require.Len(t, notifier.invoices, 1)
	inv := notifier.invoices[0]
	require.True(t, inv.IsBolt12)
	require.NotNil(t, inv.OfferID)
	require.Equal(t, offer.ID, *inv.OfferID)
	require.Equal(t,
		lnwire.MilliSatoshi(10000), inv.Terms.Value,
	)

	// Verify the preimage is set and hashes to the registered payment hash.
	require.NotNil(t, inv.Terms.PaymentPreimage)
	expectedHash := inv.Terms.PaymentPreimage.Hash()
	require.Equal(t, expectedHash, notifier.hashes[0])

	// Verify reply was sent.
	require.Len(t, replier.replies, 1)
	require.NotEmpty(t, replier.replies[0])

	// Verify the reply decodes as a valid invoice.
	replyInv, err := bolt12.DecodeInvoice(replier.replies[0])
	require.NoError(t, err)
	require.NoError(t, bolt12.VerifyInvoice(replyInv))
}

// TestHandleInvoiceRequest_NoReplyPath verifies that the handler works without
// a reply path (invoice is still registered).
func TestHandleInvoiceRequest_NoReplyPath(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	store := newMockOfferStore()
	notifier := &mockNotifier{}
	replier := &mockReplier{}

	handler := NewHandler(
		store, notifier, replier, NewPrivKeySigner(nodeKey),
		nil, testChainHash(),
	)

	offer := addOffer(t, store, nodeKey, 10000)

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	invreqBytes := buildSignedInvreqBytes(
		t, offer, payerKey, 10000,
	)

	ctx := context.Background()
	err := handler.HandleInvoiceRequest(ctx, invreqBytes, nil)
	require.NoError(t, err)

	// Invoice notification should be sent but no reply.
	require.Len(t, notifier.invoices, 1)
	require.Len(t, replier.replies, 0)
}

// TestHandleInvoiceRequest_OfferNotFound verifies that the handler returns an
// error when the offer is not in the store.
func TestHandleInvoiceRequest_OfferNotFound(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	store := newMockOfferStore()
	notifier := &mockNotifier{}
	replier := &mockReplier{}

	handler := NewHandler(
		store, notifier, replier, NewPrivKeySigner(nodeKey),
		nil, testChainHash(),
	)

	// Build a request for a non-existent offer.
	offer := testOffer(t, nodeKey, 10000)

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	invreqBytes := buildSignedInvreqBytes(
		t, offer, payerKey, 10000,
	)

	ctx := context.Background()
	err := handler.HandleInvoiceRequest(ctx, invreqBytes, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "lookup offer")
}
