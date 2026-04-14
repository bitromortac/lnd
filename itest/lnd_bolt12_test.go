package itest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lntest"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/onionmessage"
	"github.com/lightningnetwork/lnd/record"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// testCreateOffer tests that the CreateOffer RPC creates a valid BOLT 12 offer,
// persists it, and returns a decodable offer string.
func testCreateOffer(ht *lntest.HarnessTest) {
	alice := ht.NewNode("Alice", nil)

	ctxt, cancel := context.WithTimeout(
		ht.Context(), lntest.DefaultTimeout,
	)
	defer cancel()

	resp, err := alice.RPC.LN.CreateOffer(
		ctxt, &lnrpc.CreateOfferRequest{
			Description: "integration test coffee",
			AmountMsat:  10000,
		},
	)
	if err != nil && strings.Contains(
		err.Error(), "offer store not initialized",
	) {

		ht.Skipf(
			"offer store requires --dbbackend=sqlite --nativesql",
		)
	}
	require.NoError(ht, err, "CreateOffer")

	require.NotEmpty(ht, resp.Offer)

	decoded, err := bolt12.DecodeOfferString(
		resp.Offer, time.Now(), *harnessNetParams.GenesisHash,
	)
	require.NoError(ht, err, "decode offer string")

	issuerPub := decoded.OfferIssuerID.UnwrapOrFailV(ht.T)
	require.Equal(
		ht, alice.PubKey[:],
		issuerPub.SerializeCompressed(),
	)

	require.Len(ht, resp.OfferId, 32)

	tlvBytes, err := decoded.Encode()
	require.NoError(ht, err)
	expectedID := sha256.Sum256(tlvBytes)
	require.Equal(ht, expectedID[:], resp.OfferId)

	ht.Logf(
		"Created offer: %s (id=%s)", resp.Offer,
		hex.EncodeToString(resp.OfferId),
	)

	// Second offer without amount.
	resp2 := alice.RPC.CreateOffer(
		&lnrpc.CreateOfferRequest{
			Description: "tips welcome",
		},
	)
	require.NotEqual(ht, resp.OfferId, resp2.OfferId)

	decoded2, err := bolt12.DecodeOfferString(
		resp2.Offer, time.Now(), *harnessNetParams.GenesisHash,
	)
	require.NoError(ht, err)

	hasAmount := false
	decoded2.OfferAmount.WhenSome(
		func(_ tlv.RecordT[tlv.TlvType8, bolt12.TUint64]) {
			hasAmount = true
		},
	)
	require.False(ht, hasAmount)
}

// testBolt12InvoiceRequest tests the full receiver-side flow: Bob sends an
// invoice request for Alice's offer via onion message, and Alice auto-replies
// with a signed BOLT 12 invoice.
func testBolt12InvoiceRequest(ht *lntest.HarnessTest) {
	alice := ht.NewNode("Alice", nil)
	bob := ht.NewNode("Bob", nil)

	ht.EnsureConnected(alice, bob)

	// Onion message ingress is gated on the sender and receiver sharing at
	// least one fully open channel as the Sybil-resistance layer on top of
	// the byte-granular rate limiter, so open a channel before exchanging
	// invoice request / invoice messages.
	ht.FundCoins(btcutil.SatoshiPerBitcoin, bob)
	chanPoint := ht.OpenChannel(
		bob, alice, lntest.OpenChannelParams{Amt: 500_000},
	)
	defer ht.CloseChannel(bob, chanPoint)

	// Step 1: Alice creates an offer.
	ctxt, cancel := context.WithTimeout(
		ht.Context(), lntest.DefaultTimeout,
	)
	defer cancel()

	offerResp, err := alice.RPC.LN.CreateOffer(
		ctxt, &lnrpc.CreateOfferRequest{
			Description: "e2e test coffee",
			AmountMsat:  50000,
		},
	)
	if err != nil && strings.Contains(
		err.Error(), "offer store not initialized",
	) {

		ht.Skipf(
			"offer store requires --dbbackend=sqlite --nativesql",
		)
	}
	require.NoError(ht, err, "CreateOffer")
	ht.Logf("Alice offer: %s", offerResp.Offer)

	offer, err := bolt12.DecodeOfferString(
		offerResp.Offer, time.Now(), *harnessNetParams.GenesisHash,
	)
	require.NoError(ht, err)

	// Step 2: Bob subscribes to onion messages for the reply.
	msgClient, msgCancel := bob.RPC.SubscribeOnionMessages()
	defer msgCancel()

	bobMessages := make(chan *lnrpc.OnionMessageUpdate, 1)
	go func() {
		for {
			msg, recvErr := msgClient.Recv()
			if recvErr != nil {
				return
			}

			select {
			case bobMessages <- msg:
			case <-ht.Context().Done():
				return
			}
		}
	}()

	// Step 3: Build a signed invoice request.
	payerKey, err := btcec.NewPrivateKey()
	require.NoError(ht, err)

	ir := &bolt12.InvoiceRequest{}

	// Mirror offer fields.
	ir.OfferIssuerID = offer.OfferIssuerID
	ir.OfferDescription = offer.OfferDescription
	ir.OfferAmount = offer.OfferAmount
	ir.OfferFeatures = offer.OfferFeatures
	ir.OfferAbsoluteExpiry = offer.OfferAbsoluteExpiry
	ir.OfferPaths = offer.OfferPaths
	ir.OfferIssuer = offer.OfferIssuer
	ir.OfferQuantityMax = offer.OfferQuantityMax
	ir.OfferChains = offer.OfferChains
	ir.OfferMetadata = offer.OfferMetadata
	ir.OfferCurrency = offer.OfferCurrency

	// Set invreq_chain (type 80) from the offer's chains. Required on
	// non-mainnet because the codec defaults the absent field to
	// Bitcoin mainnet, which would mismatch the receiver's active
	// regtest chain.
	offer.OfferChains.WhenSome(
		func(r tlv.RecordT[tlv.TlvType2, bolt12.ChainsRecord]) {
			if len(r.Val.Chains) > 0 {
				ir.InvreqChain = tlv.SomeRecordT(
					tlv.NewPrimitiveRecord[
						tlv.TlvType80, [32]byte,
					](r.Val.Chains[0]),
				)
			}
		},
	)

	// Payer fields. invreq_payer_id is the payer's compressed pubkey.
	ir.InvreqPayerID = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType88](payerKey.PubKey()),
	)
	ir.InvreqMetadata = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType0, tlv.Blob]{
			Val: []byte("itest-payer-metadata"),
		},
	)
	amt := bolt12.TUint64(50000)
	ir.InvreqAmount = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType82, bolt12.TUint64]{
			Val: amt,
		},
	)

	// Encode (also repopulates rawTLVs) → sign → re-encode.
	if _, err := ir.Encode(); err != nil {
		require.NoError(ht, err)
	}

	sig, err := bolt12.SignInvoiceRequest(ir, payerKey)
	require.NoError(ht, err)

	ir.Signature = tlv.SomeRecordT(
		tlv.NewPrimitiveRecord[tlv.TlvType240, [64]byte](sig),
	)

	signedIRBytes, err := ir.Encode()
	require.NoError(ht, err)
	ht.Logf("Signed invreq: %d bytes", len(signedIRBytes))

	// Step 4: Build onion message to Alice with invoice request (type 64)
	// and a reply path back to Bob.
	alicePubKey, err := btcec.ParsePubKey(alice.PubKey[:])
	require.NoError(ht, err)

	bobPubKey, err := btcec.ParsePubKey(bob.PubKey[:])
	require.NoError(ht, err)

	// Blinded path to Alice (single hop: Alice is the final dest).
	aliceHops := []*sphinx.HopInfo{
		{
			NodePub: alicePubKey,
			PlainText: onionmessage.EncodeBlindedRouteData(
				ht.T, &record.BlindedRouteData{},
			),
		},
	}
	alicePath := onionmessage.BuildBlindedPath(ht.T, aliceHops)

	// Reply path to Bob (single hop: Bob is the final dest).
	bobHops := []*sphinx.HopInfo{
		{
			NodePub: bobPubKey,
			PlainText: onionmessage.EncodeBlindedRouteData(
				ht.T, &record.BlindedRouteData{},
			),
		},
	}
	bobReplyPathInfo := onionmessage.BuildBlindedPath(
		ht.T, bobHops,
	)

	// Build the onion with the reply path embedded in the final hop
	// payload.
	finalHopTLVs := []*lnwire.FinalHopTLV{
		{
			TLVType: lnwire.InvoiceRequestNamespaceType,
			Value:   signedIRBytes,
		},
	}

	replyPath, err := lnwire.NewBlindedPathFromSphinx(bobReplyPathInfo.Path)
	require.NoError(ht, err)

	sphinxPath, err := route.OnionMessageBlindedPathToSphinxPath(
		alicePath.Path,
		replyPath,
		finalHopTLVs,
	)
	require.NoError(ht, err)

	onionSessionKey, err := btcec.NewPrivateKey()
	require.NoError(ht, err)

	onionPkt, err := sphinx.NewOnionPacket(
		sphinxPath, onionSessionKey, nil,
		sphinx.DeterministicPacketFiller, sphinx.WithMaxPayloadSize(
			sphinx.MaxRoutingPayloadSize,
		),
	)
	require.NoError(ht, err)

	var buf bytes.Buffer
	require.NoError(ht, onionPkt.Encode(&buf))

	pathKey := alicePath.SessionKey.PubKey().
		SerializeCompressed()

	bob.RPC.SendOnionMessage(
		&lnrpc.SendOnionMessageRequest{
			Peer:    alice.PubKey[:],
			PathKey: pathKey,
			Onion:   buf.Bytes(),
		},
	)
	ht.Log("Bob sent invoice request to Alice")

	// Step 5: Wait for Alice's invoice reply on Bob's onion message
	// subscription.
	select {
	case msg := <-bobMessages:
		ht.Log("Bob received reply")

		// The reply should contain a type-66 (invoice) TLV.
		invoiceBytes, hasInvoice := msg.CustomRecords[uint64(
			lnwire.InvoiceNamespaceType,
		)]
		require.True(
			ht, hasInvoice,
			"reply should contain invoice (type 66)",
		)

		// Decode and verify the invoice.
		invoice, decErr := bolt12.DecodeInvoice(invoiceBytes)
		require.NoError(ht, decErr, "decode invoice")

		verifyErr := bolt12.VerifyInvoice(invoice)
		require.NoError(ht, verifyErr, "verify invoice sig")

		// invoice_node_id matches Alice.
		var nodeIDBytes []byte
		invoice.InvoiceNodeID.WhenSome(
			func(r tlv.RecordT[tlv.TlvType176, *btcec.PublicKey]) {
				if r.Val != nil {
					nodeIDBytes = r.Val.SerializeCompressed()
				}
			},
		)
		require.Equal(
			ht, alice.PubKey[:], nodeIDBytes,
			"invoice_node_id mismatch",
		)

		// invoice_amount matches 50000 msat.
		var invAmt uint64
		invoice.InvoiceAmount.WhenSome(
			func(r tlv.RecordT[tlv.TlvType170, bolt12.TUint64]) {

				invAmt = uint64(r.Val)
			},
		)
		require.Equal(
			ht, uint64(50000), invAmt, "invoice amount mismatch",
		)

		// invreq_payer_id is mirrored.
		var payerID []byte
		invoice.InvreqPayerID.WhenSome(
			func(r tlv.RecordT[tlv.TlvType88, *btcec.PublicKey]) {
				payerID = r.Val.SerializeCompressed()
			},
		)
		require.Equal(
			ht, payerKey.PubKey().SerializeCompressed(),
			payerID, "invreq_payer_id mismatch",
		)

		ht.Log("Invoice reply verified successfully")

	case <-time.After(lntest.DefaultTimeout):
		ht.Fatal("Bob did not receive invoice reply")
	}
}

// testDecodeOffer verifies the DecodeOffer RPC returns correct fields for an
// offer created by the same node.
func testDecodeOffer(ht *lntest.HarnessTest) {
	alice := ht.NewNode("Alice", nil)

	ctxt, cancel := context.WithTimeout(
		ht.Context(), lntest.DefaultTimeout,
	)
	defer cancel()

	// Create an offer.
	offerResp, err := alice.RPC.LN.CreateOffer(
		ctxt, &lnrpc.CreateOfferRequest{
			Description: "decode test",
			AmountMsat:  42000,
		},
	)
	if err != nil && strings.Contains(
		err.Error(), "offer store not initialized",
	) {

		ht.Skipf(
			"offer store requires --dbbackend=sqlite --nativesql",
		)
	}
	require.NoError(ht, err, "CreateOffer")

	// Decode the offer.
	decoded := alice.RPC.DecodeOffer(
		&lnrpc.DecodeOfferRequest{
			Offer: offerResp.Offer,
		},
	)

	require.Equal(ht, "decode test", decoded.Description)
	require.True(ht, decoded.HasAmount)
	require.Equal(ht, uint64(42000), decoded.AmountMsat)
	require.Equal(
		ht, alice.PubKey[:], decoded.OfferIssuerId,
	)
	require.Equal(ht, offerResp.OfferId, decoded.OfferId)
	require.False(ht, decoded.HasExpiry)
	require.False(ht, decoded.HasQuantityMax)

	ht.Log("DecodeOffer verified successfully")
}

// testBolt12RequestInvoiceRPC tests the full sender-side flow using the
// RequestInvoice RPC: Bob requests an invoice from Alice's offer and receives a
// validated BOLT 12 invoice.
func testBolt12RequestInvoiceRPC(ht *lntest.HarnessTest) {
	alice := ht.NewNode("Alice", nil)
	bob := ht.NewNode("Bob", nil)

	ht.EnsureConnected(alice, bob)

	// Onion message ingress is gated on the sender and receiver sharing at
	// least one fully open channel as the Sybil-resistance layer on top of
	// the byte-granular rate limiter, so open a channel before exchanging
	// invoice request / invoice messages.
	ht.FundCoins(btcutil.SatoshiPerBitcoin, bob)
	chanPoint := ht.OpenChannel(
		bob, alice, lntest.OpenChannelParams{Amt: 500_000},
	)
	defer ht.CloseChannel(bob, chanPoint)

	ctxt, cancel := context.WithTimeout(
		ht.Context(), lntest.DefaultTimeout,
	)
	defer cancel()

	// Alice creates an offer.
	offerResp, err := alice.RPC.LN.CreateOffer(
		ctxt, &lnrpc.CreateOfferRequest{
			Description: "rpc test coffee",
			AmountMsat:  75000,
		},
	)
	if err != nil && strings.Contains(
		err.Error(), "offer store not initialized",
	) {

		ht.Skipf(
			"offer store requires --dbbackend=sqlite --nativesql",
		)
	}
	require.NoError(ht, err, "CreateOffer")
	ht.Logf("Alice offer: %s", offerResp.Offer)

	// Bob requests an invoice for Alice's offer.
	invoiceResp := bob.RPC.RequestInvoice(
		&lnrpc.RequestInvoiceRequest{
			Offer:          offerResp.Offer,
			TimeoutSeconds: 30,
		},
	)
	require.NotEmpty(ht, invoiceResp.InvoiceString)
	require.Equal(
		ht, alice.PubKey[:], invoiceResp.InvoiceNodeId,
		"invoice_node_id should match Alice",
	)
	require.Equal(
		ht, uint64(75000), invoiceResp.InvoiceAmountMsat,
		"invoice amount should match offer",
	)
	require.Len(
		ht, invoiceResp.InvoicePaymentHash, 32,
		"payment hash should be 32 bytes",
	)
	require.Len(
		ht, invoiceResp.InvreqPayerId, 33,
		"payer ID should be 33 bytes",
	)

	// Verify the embedded offer fields.
	require.NotNil(ht, invoiceResp.Offer)
	require.Equal(
		ht, "rpc test coffee",
		invoiceResp.Offer.Description,
	)
	require.Equal(
		ht, uint64(75000),
		invoiceResp.Offer.AmountMsat,
	)

	// Verify the invoice string decodes.
	inv, decErr := bolt12.DecodeInvoiceString(
		invoiceResp.InvoiceString, time.Now(),
		*harnessNetParams.GenesisHash,
	)
	require.NoError(ht, decErr, "decode invoice string")
	require.NoError(
		ht, bolt12.VerifyInvoice(inv), "verify invoice signature",
	)

	ht.Log("RequestInvoice RPC verified successfully")
}

// testBolt12PayOffer tests the full end-to-end BOLT 12 payment flow: Alice
// creates an offer, Bob pays it via PayOffer, and both sides see the settled
// payment.
func testBolt12PayOffer(ht *lntest.HarnessTest) {
	alice := ht.NewNode("Alice", nil)
	bob := ht.NewNode("Bob", nil)

	ht.EnsureConnected(alice, bob)

	// Fund Bob so he can open a channel to Alice.
	ht.FundCoins(btcutil.SatoshiPerBitcoin, bob)

	// Open a channel from Bob to Alice so Bob can pay.
	chanPoint := ht.OpenChannel(
		bob, alice, lntest.OpenChannelParams{
			Amt: 500_000,
		},
	)
	defer ht.CloseChannel(bob, chanPoint)

	ctxt, cancel := context.WithTimeout(
		ht.Context(), lntest.DefaultTimeout,
	)
	defer cancel()

	// Alice creates an offer.
	offerResp, err := alice.RPC.LN.CreateOffer(
		ctxt, &lnrpc.CreateOfferRequest{
			Description: "pay offer test",
			AmountMsat:  50000,
		},
	)
	if err != nil && strings.Contains(
		err.Error(), "offer store not initialized",
	) {

		ht.Skipf(
			"offer store requires --dbbackend=sqlite --nativesql",
		)
	}
	require.NoError(ht, err, "CreateOffer")
	ht.Logf("Alice offer: %s", offerResp.Offer)

	// Stateless settlement: Alice should have zero BOLT 12 invoices before
	// any payment. The invoice is only reconstructed at HTLC arrival time.
	listBefore, err := alice.RPC.LN.ListInvoices(
		ctxt, &lnrpc.ListInvoiceRequest{},
	)
	require.NoError(ht, err)

	bolt12CountBefore := 0
	for _, inv := range listBefore.Invoices {
		if inv.IsBolt12 {
			bolt12CountBefore++
		}
	}
	require.Equal(
		ht, 0, bolt12CountBefore,
		"stateless: no BOLT 12 invoices should exist before payment",
	)

	// Bob pays Alice's offer.
	payResp := bob.RPC.PayOffer(
		&lnrpc.PayOfferRequest{
			Offer:          offerResp.Offer,
			TimeoutSeconds: 30,
		},
	)

	// Verify the payment preimage is 32 bytes and non-zero.
	require.Len(ht, payResp.PaymentPreimage, 32)
	require.NotEqual(
		ht, make([]byte, 32), payResp.PaymentPreimage,
	)

	// Verify the payment hash is 32 bytes.
	require.Len(ht, payResp.PaymentHash, 32)

	// Verify the settled amount matches the offer.
	require.Equal(
		ht, uint64(50000), payResp.AmountMsat,
		"settled amount should match offer",
	)

	// Verify payer ID is present (33-byte compressed pubkey).
	require.Len(ht, payResp.InvreqPayerId, 33)

	// Verify the embedded offer fields.
	require.NotNil(ht, payResp.Offer)
	require.Equal(ht, "pay offer test", payResp.Offer.Description)
	require.Equal(
		ht, uint64(50000), payResp.Offer.AmountMsat,
	)

	// Verify the invoice string decodes.
	inv, decErr := bolt12.DecodeInvoiceString(
		payResp.InvoiceString, time.Now(),
		*harnessNetParams.GenesisHash,
	)
	require.NoError(ht, decErr, "decode invoice string")
	require.NoError(
		ht, bolt12.VerifyInvoice(inv), "verify invoice signature",
	)

	// --- Layer 6: Verify ListInvoices BOLT 12 fields ---

	// Alice (receiver) lists invoices and verifies the BOLT 12
	// invoice has the OfferInvoiceDetail populated.
	listInvResp, err := alice.RPC.LN.ListInvoices(
		ctxt, &lnrpc.ListInvoiceRequest{},
	)
	require.NoError(ht, err, "ListInvoices")

	// Find the BOLT 12 invoice.
	var bolt12Inv *lnrpc.Invoice
	for _, rpcInv := range listInvResp.Invoices {
		if rpcInv.IsBolt12 {
			bolt12Inv = rpcInv
			break
		}
	}
	require.NotNil(ht, bolt12Inv, "BOLT 12 invoice in ListInvoices")
	require.NotNil(
		ht, bolt12Inv.Bolt12Detail,
		"bolt12_detail should be populated",
	)
	require.Len(
		ht, bolt12Inv.Bolt12Detail.OfferId, 32,
		"offer_id should be 32 bytes",
	)
	require.Equal(
		ht, offerResp.OfferId, bolt12Inv.Bolt12Detail.OfferId,
		"offer_id should match CreateOffer response",
	)
	require.Len(
		ht, bolt12Inv.Bolt12Detail.InvoiceNodeId, 33,
		"invoice_node_id should be 33 bytes",
	)
	require.Len(
		ht, bolt12Inv.Bolt12Detail.InvreqPayerId, 33,
		"invreq_payer_id should be 33 bytes",
	)

	// --- Layer 6: Verify ListPayments BOLT 12 fields ---

	// Bob (sender) lists payments and verifies offer_id.
	listPayResp, err := bob.RPC.LN.ListPayments(
		ctxt, &lnrpc.ListPaymentsRequest{
			IncludeIncomplete: true,
		},
	)
	require.NoError(ht, err, "ListPayments")

	// Find the payment with a non-empty offer_id.
	var bolt12Pay *lnrpc.Payment
	for _, rpcPay := range listPayResp.Payments {
		if len(rpcPay.OfferId) > 0 {
			bolt12Pay = rpcPay
			break
		}
	}
	require.NotNil(ht, bolt12Pay, "BOLT 12 payment in ListPayments")
	require.Equal(
		ht, offerResp.OfferId, bolt12Pay.OfferId,
		"payment offer_id should match CreateOffer response",
	)

	// Verify ListPayments offer_id filter.
	filteredResp, err := bob.RPC.LN.ListPayments(
		ctxt, &lnrpc.ListPaymentsRequest{
			IncludeIncomplete: true,
			OfferId:           offerResp.OfferId,
		},
	)
	require.NoError(ht, err, "ListPayments with offer_id filter")
	require.Len(
		ht, filteredResp.Payments, 1,
		"offer_id filter should return exactly one payment",
	)
	require.Equal(
		ht, offerResp.OfferId, filteredResp.Payments[0].OfferId,
	)

	ht.Log("PayOffer verified successfully")
}

// testBolt12PayOfferNoAmount tests PayOffer with an offer that has no fixed
// amount — the sender specifies the amount.
func testBolt12PayOfferNoAmount(ht *lntest.HarnessTest) {
	alice := ht.NewNode("Alice", nil)
	bob := ht.NewNode("Bob", nil)

	ht.EnsureConnected(alice, bob)

	ht.FundCoins(btcutil.SatoshiPerBitcoin, bob)

	chanPoint := ht.OpenChannel(
		bob, alice, lntest.OpenChannelParams{
			Amt: 500_000,
		},
	)
	defer ht.CloseChannel(bob, chanPoint)

	ctxt, cancel := context.WithTimeout(
		ht.Context(), lntest.DefaultTimeout,
	)
	defer cancel()

	// Alice creates a no-amount offer.
	offerResp, err := alice.RPC.LN.CreateOffer(
		ctxt, &lnrpc.CreateOfferRequest{
			Description: "tips welcome",
		},
	)
	if err != nil && strings.Contains(
		err.Error(), "offer store not initialized",
	) {

		ht.Skipf(
			"offer store requires --dbbackend=sqlite --nativesql",
		)
	}
	require.NoError(ht, err, "CreateOffer")

	// Bob pays with a sender-specified amount.
	payResp := bob.RPC.PayOffer(
		&lnrpc.PayOfferRequest{
			Offer:          offerResp.Offer,
			AmountMsat:     25000,
			TimeoutSeconds: 30,
		},
	)

	require.Len(ht, payResp.PaymentPreimage, 32)
	require.Equal(
		ht, uint64(25000), payResp.AmountMsat,
		"settled amount should match sender request",
	)

	ht.Log("PayOffer no-amount verified successfully")
}

// testBolt12PayOfferMultiHop tests the full end-to-end multi-hop BOLT 12 flow
// with no direct channel between sender and receiver.
//
// Topology (4-node chain):
//
//	Carol → Bob → Dave → Alice
//
// Carol (sender) pays Alice's (receiver) offer. Everything is multi-hop:
//   - Forward onion message: Carol → Bob → Dave → Alice (BFS pathfinding)
//   - Reply path: Bob (intro) → Carol (DFS blinded message path)
//   - Invoice blinded payment path: Dave (intro) → Alice (DFS blinded payment
//     path)
//   - HTLC payment: Carol → Bob → Dave (cleartext) → Alice (blinded)
func testBolt12PayOfferMultiHop(ht *lntest.HarnessTest) {
	// Create a 4-node chain: Carol → Bob → Dave → Alice.
	// CreateSimpleNetwork opens channels left-to-right and funds the
	// opener, so Carol has outbound to Bob, Bob to Dave, Dave to Alice.
	chanPoints, nodes := ht.CreateSimpleNetwork(
		[][]string{nil, nil, nil, nil},
		lntest.OpenChannelParams{Amt: 500_000},
	)
	defer func() {
		for i := len(chanPoints) - 1; i >= 0; i-- {
			ht.CloseChannel(nodes[i], chanPoints[i])
		}
	}()

	carol := nodes[0]
	alice := nodes[3]

	ctxt, cancel := context.WithTimeout(
		ht.Context(), lntest.DefaultTimeout,
	)
	defer cancel()

	// Alice creates an offer.
	offerResp, err := alice.RPC.LN.CreateOffer(
		ctxt, &lnrpc.CreateOfferRequest{
			Description: "multi-hop e2e test",
			AmountMsat:  50000,
		},
	)
	if err != nil && strings.Contains(
		err.Error(), "offer store not initialized",
	) {

		ht.Skipf(
			"offer store requires --dbbackend=sqlite --nativesql",
		)
	}
	require.NoError(ht, err, "CreateOffer")
	ht.Logf("Alice offer: %s", offerResp.Offer)

	// Carol pays Alice's offer through the full 4-node chain.
	// No direct channel between Carol and Alice — everything routes
	// through Bob and Dave.
	payResp := carol.RPC.PayOffer(
		&lnrpc.PayOfferRequest{
			Offer:          offerResp.Offer,
			TimeoutSeconds: 60,
			FeeLimitMsat:   50000,
		},
	)

	// Verify the payment settled correctly.
	require.Len(ht, payResp.PaymentPreimage, 32)
	require.NotEqual(
		ht, make([]byte, 32), payResp.PaymentPreimage,
	)
	require.Equal(
		ht, uint64(50000), payResp.AmountMsat,
		"settled amount should match offer",
	)

	ht.Log("PayOffer multi-hop e2e verified successfully")
}

// testBolt12PayOfferBlindedOffer tests the full end-to-end BOLT 12 flow using
// offer_paths instead of offer_issuer_id. The receiver's identity is hidden
// behind blinded message paths in the offer.
//
// Topology (4-node chain):
//
//	Carol → Bob → Dave → Alice
//
// Alice creates an offer with use_blinded_paths=true. The offer contains
// blinded message paths (offer_paths) instead of Alice's pubkey. Carol sends
// the invoice request through the offer's blinded path to reach Alice.
func testBolt12PayOfferBlindedOffer(ht *lntest.HarnessTest) {
	chanPoints, nodes := ht.CreateSimpleNetwork(
		[][]string{nil, nil, nil, nil},
		lntest.OpenChannelParams{Amt: 500_000},
	)
	defer func() {
		for i := len(chanPoints) - 1; i >= 0; i-- {
			ht.CloseChannel(nodes[i], chanPoints[i])
		}
	}()

	carol := nodes[0]
	alice := nodes[3]

	ctxt, cancel := context.WithTimeout(
		ht.Context(), lntest.DefaultTimeout,
	)
	defer cancel()

	// Alice creates an offer with blinded paths (no offer_issuer_id).
	offerResp, err := alice.RPC.LN.CreateOffer(
		ctxt, &lnrpc.CreateOfferRequest{
			Description:     "private offer",
			AmountMsat:      50000,
			UseBlindedPaths: true,
		},
	)
	if err != nil && strings.Contains(
		err.Error(), "offer store not initialized",
	) {

		ht.Skipf(
			"offer store requires --dbbackend=sqlite --nativesql",
		)
	}
	require.NoError(ht, err, "CreateOffer with blinded paths")
	ht.Logf("Alice blinded offer: %s", offerResp.Offer)

	// Decode the offer to verify it has offer_paths and no
	// offer_issuer_id.
	offer, err := bolt12.DecodeOfferString(
		offerResp.Offer, time.Now(), *harnessNetParams.GenesisHash,
	)
	require.NoError(ht, err, "decode offer")

	hasIssuerID := false
	offer.OfferIssuerID.WhenSome(
		func(_ tlv.RecordT[tlv.TlvType22, *btcec.PublicKey]) {
			hasIssuerID = true
		},
	)
	require.False(ht, hasIssuerID,
		"blinded offer should not have offer_issuer_id")

	hasPaths := false
	offer.OfferPaths.WhenSome(
		func(_ tlv.RecordT[tlv.TlvType16, lnwire.BlindedPaths]) {
			hasPaths = true
		},
	)
	require.True(ht, hasPaths,
		"blinded offer should have offer_paths")

	// Carol pays Alice's blinded offer.
	payResp := carol.RPC.PayOffer(
		&lnrpc.PayOfferRequest{
			Offer:          offerResp.Offer,
			TimeoutSeconds: 60,
			FeeLimitMsat:   50000,
		},
	)

	require.Len(ht, payResp.PaymentPreimage, 32)
	require.NotEqual(
		ht, make([]byte, 32), payResp.PaymentPreimage,
	)
	require.Equal(
		ht, uint64(50000), payResp.AmountMsat,
		"settled amount should match offer",
	)

	ht.Log("PayOffer with blinded offer_paths verified successfully")
}

// testBolt12PayOfferDedup tests the offer-level dedup guard: paying the
// same offer twice without --force is rejected, with --force succeeds.
func testBolt12PayOfferDedup(ht *lntest.HarnessTest) {
	alice := ht.NewNode("Alice", nil)
	bob := ht.NewNode("Bob", nil)

	ht.EnsureConnected(alice, bob)
	ht.FundCoins(btcutil.SatoshiPerBitcoin, bob)

	chanPoint := ht.OpenChannel(
		bob, alice, lntest.OpenChannelParams{
			Amt: 500_000,
		},
	)
	defer ht.CloseChannel(bob, chanPoint)

	ctxt, cancel := context.WithTimeout(
		ht.Context(), lntest.DefaultTimeout,
	)
	defer cancel()

	// Alice creates an offer.
	offerResp, err := alice.RPC.LN.CreateOffer(
		ctxt, &lnrpc.CreateOfferRequest{
			Description: "dedup test",
			AmountMsat:  10000,
		},
	)
	if err != nil && strings.Contains(
		err.Error(), "offer store not initialized",
	) {
		ht.Skipf(
			"offer store requires --dbbackend=sqlite " +
				"--nativesql",
		)
	}
	require.NoError(ht, err, "CreateOffer")

	// First payment succeeds.
	payResp := bob.RPC.PayOffer(
		&lnrpc.PayOfferRequest{
			Offer:          offerResp.Offer,
			TimeoutSeconds: 30,
		},
	)
	require.Len(ht, payResp.PaymentPreimage, 32)

	// Second payment without --force should fail.
	stream, err := bob.RPC.LN.PayOffer(
		ctxt, &lnrpc.PayOfferRequest{
			Offer:          offerResp.Offer,
			TimeoutSeconds: 30,
		},
	)
	require.NoError(ht, err, "PayOffer stream open")

	_, recvErr := stream.Recv()
	require.Error(ht, recvErr, "expected dedup error")
	require.Contains(
		ht, recvErr.Error(), "already exists",
	)

	// Third payment with --force succeeds.
	payResp2 := bob.RPC.PayOffer(
		&lnrpc.PayOfferRequest{
			Offer:          offerResp.Offer,
			TimeoutSeconds: 30,
			Force:          true,
		},
	)
	require.Len(ht, payResp2.PaymentPreimage, 32)

	ht.Log("PayOffer dedup verified successfully")
}

// testBolt12PayOfferStreamEvents tests that the PayOffer stream
// emits all three update types in the correct order.
func testBolt12PayOfferStreamEvents(ht *lntest.HarnessTest) {
	alice := ht.NewNode("Alice", nil)
	bob := ht.NewNode("Bob", nil)

	ht.EnsureConnected(alice, bob)
	ht.FundCoins(btcutil.SatoshiPerBitcoin, bob)

	chanPoint := ht.OpenChannel(
		bob, alice, lntest.OpenChannelParams{
			Amt: 500_000,
		},
	)
	defer ht.CloseChannel(bob, chanPoint)

	ctxt, cancel := context.WithTimeout(
		ht.Context(), lntest.DefaultTimeout,
	)
	defer cancel()

	offerResp, err := alice.RPC.LN.CreateOffer(
		ctxt, &lnrpc.CreateOfferRequest{
			Description: "stream events test",
			AmountMsat:  20000,
		},
	)
	if err != nil && strings.Contains(
		err.Error(), "offer store not initialized",
	) {
		ht.Skipf(
			"offer store requires --dbbackend=sqlite " +
				"--nativesql",
		)
	}
	require.NoError(ht, err, "CreateOffer")

	// Open stream directly (not via harness helper).
	stream, err := bob.RPC.LN.PayOffer(
		ctxt, &lnrpc.PayOfferRequest{
			Offer:          offerResp.Offer,
			TimeoutSeconds: 30,
		},
	)
	require.NoError(ht, err, "PayOffer stream open")

	// First update: invoice_request_sent.
	update1, err := stream.Recv()
	require.NoError(ht, err, "recv update 1")
	reqSent := update1.GetInvoiceRequestSent()
	require.NotNil(ht, reqSent, "expected invoice_request_sent")
	require.NotNil(ht, reqSent.Offer)

	// Second update: invoice_received.
	update2, err := stream.Recv()
	require.NoError(ht, err, "recv update 2")
	invRecv := update2.GetInvoiceReceived()
	require.NotNil(ht, invRecv, "expected invoice_received")
	require.NotEmpty(ht, invRecv.InvoiceString)
	require.Len(ht, invRecv.InvoicePaymentHash, 32)
	require.Equal(ht, uint64(20000), invRecv.InvoiceAmountMsat)

	// Third update: payment_result.
	update3, err := stream.Recv()
	require.NoError(ht, err, "recv update 3")
	result := update3.GetPaymentResult()
	require.NotNil(ht, result, "expected payment_result")
	require.Len(ht, result.PaymentPreimage, 32)
	require.NotEqual(
		ht, make([]byte, 32), result.PaymentPreimage,
	)
	require.Equal(ht, uint64(20000), result.AmountMsat)

	ht.Log("PayOffer stream events verified successfully")
}

// testBolt12PayOfferMPP tests BOLT 12 payment over multiple paths. Alice
// creates an offer, Bob has two smaller channels to Alice so that the payment
// forces MPP splitting. This verifies that the stateless reconstruction handles
// multi-shard correctly (first shard INSERTs, subsequent shards accumulate).
func testBolt12PayOfferMPP(ht *lntest.HarnessTest) {
	alice := ht.NewNode("Alice", nil)
	bob := ht.NewNode("Bob", nil)

	ht.EnsureConnected(alice, bob)

	ht.FundCoins(btcutil.SatoshiPerBitcoin, bob)

	// Open two channels from Bob to Alice sized so that a 150k-sat
	// payment exceeds either channel's per-shard usable balance
	// (commit fees + 1% reserve consume ~25k per 200k channel) but
	// fits comfortably within the combined liquidity. This forces
	// the sender into MPP splitting.
	chanPoint1 := ht.OpenChannel(
		bob, alice, lntest.OpenChannelParams{
			Amt: 200_000,
		},
	)
	defer ht.CloseChannel(bob, chanPoint1)

	chanPoint2 := ht.OpenChannel(
		bob, alice, lntest.OpenChannelParams{
			Amt: 200_000,
		},
	)
	defer ht.CloseChannel(bob, chanPoint2)

	// Wait for both channels to be visible in Bob's routing graph
	// before paying so the sender's pathfinder can split across them.
	ht.AssertChannelInGraph(bob, chanPoint1)
	ht.AssertChannelInGraph(bob, chanPoint2)

	ctxt, cancel := context.WithTimeout(
		ht.Context(), lntest.DefaultTimeout,
	)
	defer cancel()

	// Alice creates an offer. The amount must exceed the per-channel
	// usable balance (~190k sat after commit fee + 1% reserve on a
	// 200k channel) so the sender is forced to split the payment.
	offerResp, err := alice.RPC.LN.CreateOffer(
		ctxt, &lnrpc.CreateOfferRequest{
			Description: "mpp test",
			AmountMsat:  250_000_000, // 250k sat in msat
		},
	)
	if err != nil && strings.Contains(
		err.Error(), "offer store not initialized",
	) {

		ht.Skipf(
			"offer store requires --dbbackend=sqlite --nativesql",
		)
	}
	require.NoError(ht, err, "CreateOffer")

	// Bob pays with an amount exceeding single-channel capacity,
	// forcing MPP.
	payResp := bob.RPC.PayOffer(
		&lnrpc.PayOfferRequest{
			Offer:          offerResp.Offer,
			TimeoutSeconds: 60,
		},
	)
	require.Len(ht, payResp.PaymentPreimage, 32)
	require.Equal(
		ht, uint64(250_000_000), payResp.AmountMsat,
		"settled amount should match offer",
	)

	// Verify Alice has exactly one settled BOLT 12 invoice.
	listResp, err := alice.RPC.LN.ListInvoices(
		ctxt, &lnrpc.ListInvoiceRequest{},
	)
	require.NoError(ht, err)

	var bolt12Inv *lnrpc.Invoice
	for _, inv := range listResp.Invoices {
		if inv.IsBolt12 {
			bolt12Inv = inv
		}
	}
	require.NotNil(ht, bolt12Inv, "BOLT 12 invoice after MPP payment")
	require.True(ht, bolt12Inv.Settled)
	require.Equal(
		ht, int64(250_000_000), bolt12Inv.AmtPaidMsat,
		"settled amount should match",
	)

	// Verify Bob's payment shows multiple HTLC attempts.
	listPayResp, err := bob.RPC.LN.ListPayments(
		ctxt, &lnrpc.ListPaymentsRequest{},
	)
	require.NoError(ht, err)

	var mppPayment *lnrpc.Payment
	for _, p := range listPayResp.Payments {
		if len(p.Htlcs) > 1 {
			mppPayment = p
		}
	}
	require.NotNil(
		ht, mppPayment,
		"payment should have multiple HTLC attempts (MPP)",
	)

	ht.Log("BOLT 12 MPP payment verified with stateless settlement")
}
