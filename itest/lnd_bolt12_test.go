package itest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
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
