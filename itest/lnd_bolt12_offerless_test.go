package itest

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lntest"
	"github.com/lightningnetwork/lnd/lntest/node"
	"github.com/lightningnetwork/lnd/lntest/wait"
	"github.com/stretchr/testify/require"
)

// offerlessNetwork is a payer Alice with a channel to Bob, who has a channel
// to the payee Carol. Dave has a channel to Bob and plays a node that answers
// a request it was not meant for. Bob takes onion messages only from channel
// peers. Alice's invreq_paths start at Bob.
type offerlessNetwork struct {
	alice, bob, carol, dave *node.HarnessNode
}

// newOfferlessNetwork builds the network and returns a function that closes
// its channels.
func newOfferlessNetwork(ht *lntest.HarnessTest) (*offerlessNetwork, func()) {
	args := bolt12NodeArgs(ht)
	chanPoints, nodes := ht.CreateSimpleNetwork(
		[][]string{args, args, args},
		lntest.OpenChannelParams{Amt: 500_000},
	)

	dave := ht.NewNode("Dave", args)
	ht.EnsureConnected(dave, nodes[1])
	ht.FundCoins(btcutil.SatoshiPerBitcoin, dave)
	daveChan := ht.OpenChannel(
		dave, nodes[1], lntest.OpenChannelParams{Amt: 500_000},
	)

	closeChannels := func() {
		ht.CloseChannel(dave, daveChan)
		for i := len(chanPoints) - 1; i >= 0; i-- {
			ht.CloseChannel(nodes[i], chanPoints[i])
		}
	}

	return &offerlessNetwork{
		alice: nodes[0],
		bob:   nodes[1],
		carol: nodes[2],
		dave:  dave,
	}, closeChannels
}

// fetchPublishedRequest returns Alice's published request with the given
// idempotency key.
func fetchPublishedRequest(ht *lntest.HarnessTest, alice *node.HarnessNode,
	key []byte) *lnrpc.PublishedInvoiceRequest {

	resp, err := alice.RPC.LN.ListInvoiceRequests(
		ht.Context(), &lnrpc.ListInvoiceRequestsRequest{},
	)
	require.NoError(ht, err)

	for _, req := range resp.InvoiceRequests {
		if bytes.Equal(req.IdempotencyKey, key) {
			return req
		}
	}
	require.Fail(ht, "published request not found")

	return nil
}

// sendOfferlessInvoice lets the payee answer the request and returns the
// payment hash of the stored invoice.
func sendOfferlessInvoice(ht *lntest.HarnessTest, payee *node.HarnessNode,
	lnr string) []byte {

	resp, err := payee.RPC.LN.SendInvoice(
		ht.Context(), &lnrpc.SendInvoiceRequest{InvoiceRequest: lnr},
	)
	require.NoError(ht, err)
	require.Equal(ht, uint64(1_000_000), resp.AmountMsat)

	return resp.PaymentHash
}

// testBolt12OfferlessExpectedNode verifies an invoice request without an
// offer whose expected node is known: an invoice from another node is not
// paid, the invoice from the expected node is paid at once, and the request
// pays only once.
func testBolt12OfferlessExpectedNode(ht *lntest.HarnessTest) {
	net, closeChannels := newOfferlessNetwork(ht)
	defer closeChannels()

	alice, carol, dave := net.alice, net.carol, net.dave

	created, err := alice.RPC.LN.CreateInvoiceRequest(
		ht.Context(), &lnrpc.CreateInvoiceRequestRequest{
			Description:    "refund",
			AmountMsat:     1_000_000,
			ExpectedNodeId: carol.PubKey[:],
		},
	)
	require.NoError(ht, err)

	ir, err := bolt12.DecodeInvoiceRequestString(
		created.InvoiceRequest, *harnessNetParams.GenesisHash,
	)
	require.NoError(ht, err)
	require.False(ht, ir.OfferIssuerID.IsSome())
	require.True(ht, ir.InvreqPaths.IsSome())

	// The same key returns the same request, and another amount with the
	// key is refused.
	again, err := alice.RPC.LN.CreateInvoiceRequest(
		ht.Context(), &lnrpc.CreateInvoiceRequestRequest{
			Description:    "refund",
			AmountMsat:     1_000_000,
			ExpectedNodeId: carol.PubKey[:],
			IdempotencyKey: created.IdempotencyKey,
		},
	)
	require.NoError(ht, err)
	require.Equal(ht, created.InvoiceRequest, again.InvoiceRequest)

	_, err = alice.RPC.LN.CreateInvoiceRequest(
		ht.Context(), &lnrpc.CreateInvoiceRequestRequest{
			Description:    "refund",
			AmountMsat:     2_000_000,
			ExpectedNodeId: carol.PubKey[:],
			IdempotencyKey: created.IdempotencyKey,
		},
	)
	require.Error(ht, err)

	// Dave answers first. His invoice is valid, but it does not come
	// from the expected node, so Alice neither pays it nor keeps it.
	daveHash := sendOfferlessInvoice(ht, dave, created.InvoiceRequest)

	// Carol answers, and Alice pays her invoice at once.
	carolHash := sendOfferlessInvoice(ht, carol, created.InvoiceRequest)

	err = wait.NoError(func() error {
		inv := carol.RPC.LookupInvoice(carolHash)
		if inv.State != lnrpc.Invoice_SETTLED {
			return fmt.Errorf("invoice state %v", inv.State)
		}

		return nil
	}, lntest.DefaultTimeout)
	require.NoError(ht, err, "Carol's invoice not settled")

	published := fetchPublishedRequest(ht, alice, created.IdempotencyKey)
	require.True(ht, published.Used)
	require.Equal(ht, carolHash, published.PaymentHash)
	require.Empty(ht, published.PendingInvoices)

	// A second invoice from Carol is not paid: the request paid once.
	secondHash := sendOfferlessInvoice(ht, carol, created.InvoiceRequest)
	time.Sleep(2 * time.Second)

	second := carol.RPC.LookupInvoice(secondHash)
	require.Equal(ht, lnrpc.Invoice_OPEN, second.State)

	daveInv := dave.RPC.LookupInvoice(daveHash)
	require.Equal(ht, lnrpc.Invoice_OPEN, daveInv.State)
}

// testBolt12OfferlessApproval verifies an invoice request without an
// expected node: each valid invoice waits for approval, the approved one is
// paid, and the request then drops the others.
func testBolt12OfferlessApproval(ht *lntest.HarnessTest) {
	net, closeChannels := newOfferlessNetwork(ht)
	defer closeChannels()

	alice, carol, dave := net.alice, net.carol, net.dave

	created, err := alice.RPC.LN.CreateInvoiceRequest(
		ht.Context(), &lnrpc.CreateInvoiceRequestRequest{
			Description: "withdrawal",
			AmountMsat:  1_000_000,
		},
	)
	require.NoError(ht, err)

	daveHash := sendOfferlessInvoice(ht, dave, created.InvoiceRequest)
	carolHash := sendOfferlessInvoice(ht, carol, created.InvoiceRequest)

	// Both invoices wait for approval, and nothing is paid.
	err = wait.NoError(func() error {
		published := fetchPublishedRequest(
			ht, alice, created.IdempotencyKey,
		)
		if len(published.PendingInvoices) != 2 {
			return fmt.Errorf("%d pending invoices",
				len(published.PendingInvoices))
		}

		return nil
	}, lntest.DefaultTimeout)
	require.NoError(ht, err)

	published := fetchPublishedRequest(ht, alice, created.IdempotencyKey)
	require.False(ht, published.Used)
	for _, pending := range published.PendingInvoices {
		switch {
		case bytes.Equal(pending.PaymentHash, carolHash):
			require.Equal(
				ht, carol.PubKey[:], pending.InvoiceNodeId,
			)

		case bytes.Equal(pending.PaymentHash, daveHash):
			require.Equal(
				ht, dave.PubKey[:], pending.InvoiceNodeId,
			)

		default:
			require.Fail(ht, "unknown pending invoice")
		}
	}

	ctxt, cancel := context.WithTimeout(
		ht.Context(), lntest.DefaultTimeout,
	)
	defer cancel()

	// An unknown hash is refused.
	_, err = alice.RPC.LN.ApproveInvoiceRequestPayment(
		ctxt, &lnrpc.ApproveInvoiceRequestPaymentRequest{
			PaymentHash: bytes.Repeat([]byte{1}, 32),
		},
	)
	require.Error(ht, err)

	approved, err := alice.RPC.LN.ApproveInvoiceRequestPayment(
		ctxt, &lnrpc.ApproveInvoiceRequestPaymentRequest{
			PaymentHash: carolHash,
		},
	)
	require.NoError(ht, err)
	require.Len(ht, approved.PaymentPreimage, 32)

	carolInv := carol.RPC.LookupInvoice(carolHash)
	require.Equal(ht, lnrpc.Invoice_SETTLED, carolInv.State)

	// The request paid, so Dave's invoice is gone and cannot be approved.
	published = fetchPublishedRequest(ht, alice, created.IdempotencyKey)
	require.True(ht, published.Used)
	require.Equal(ht, carolHash, published.PaymentHash)
	require.Empty(ht, published.PendingInvoices)

	_, err = alice.RPC.LN.ApproveInvoiceRequestPayment(
		ctxt, &lnrpc.ApproveInvoiceRequestPaymentRequest{
			PaymentHash: daveHash,
		},
	)
	require.Error(ht, err)
}
