package bolt12handler

import (
	"context"
	"testing"
	"time"

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

	m.offers[offer.Hash] = offer
	offer.ID = int64(len(m.offers))

	return offer.ID, nil
}

func (m *mockOfferStore) GetOfferByHash(_ context.Context,
	offerHash [32]byte) (*offers.Offer, error) {

	if o, ok := m.offers[offerHash]; ok {
		return o, nil
	}

	return nil, offers.ErrOfferNotFound
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

// addOffer creates a bolt12 offer and stores it in the mock store. It returns
// the codec offer and the stored record.
func addOffer(t *testing.T, store *mockOfferStore, nodeKey *btcec.PrivateKey,
	amountMsat uint64) (*bolt12.Offer, *offers.Offer) {

	t.Helper()

	b12Offer := testOffer(t, nodeKey, amountMsat)

	encoded, err := bolt12.EncodeOfferString(b12Offer)
	require.NoError(t, err)

	offerHash, err := bolt12.OfferHash(b12Offer)
	require.NoError(t, err)

	offer := &offers.Offer{
		Hash:    offerHash,
		Encoded: encoded,
	}

	_, err = store.InsertOffer(t.Context(), offer)
	require.NoError(t, err)

	return b12Offer, offer
}

// buildSignedInvreqBytes constructs a signed invoice request and returns the
// raw TLV bytes.
func buildSignedInvreqBytes(t *testing.T, offer *bolt12.Offer,
	payerKey *btcec.PrivateKey, invreqAmount uint64) []byte {

	t.Helper()

	ir := testInvoiceRequest(t, offer, payerKey, invreqAmount)

	sig, err := bolt12.SignInvoiceRequest(ir, payerKey)
	require.NoError(t, err)

	ir.Signature = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType240, [64]byte]{
			Val: sig,
		},
	)

	finalBytes, err := ir.EncodeSigned()
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
	offer, stored := addOffer(t, store, nodeKey, 10000)

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
	ctx := t.Context()
	err := handler.HandleInvoiceRequest(
		ctx, invreqBytes, replyPath, nil,
	)
	require.NoError(t, err)

	// Verify invoice notification was sent (no DB write).
	require.Len(t, notifier.invoices, 1)
	inv := notifier.invoices[0]
	require.True(t, inv.IsBolt12)
	require.NotNil(t, inv.OfferID)
	require.Equal(t, stored.ID, *inv.OfferID)
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

	// Verify the reply decodes as an invoice a payer would accept, which
	// covers the signature, the mirror against the request and expiry.
	replyInv, err := bolt12.DecodeInvoice(replier.replies[0])
	require.NoError(t, err)

	sentReq, err := bolt12.DecodeInvoiceRequest(invreqBytes)
	require.NoError(t, err)

	require.NoError(t, ValidateInvoiceReply(
		replyInv, sentReq, nil, testChainHash(), time.Now(),
	))
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

	offer, _ := addOffer(t, store, nodeKey, 10000)

	var payerSeed [32]byte
	payerSeed[0] = 0xFF
	payerKey, _ := btcec.PrivKeyFromBytes(payerSeed[:])

	invreqBytes := buildSignedInvreqBytes(
		t, offer, payerKey, 10000,
	)

	ctx := t.Context()
	err := handler.HandleInvoiceRequest(ctx, invreqBytes, nil, nil)
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

	ctx := t.Context()
	err := handler.HandleInvoiceRequest(ctx, invreqBytes, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "lookup offer")
}
