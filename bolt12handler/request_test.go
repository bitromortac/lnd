package bolt12handler

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// TestBuildInvoiceRequest_BasicOffer verifies that an invoice request is
// correctly constructed from an offer with a fixed amount.
func TestBuildInvoiceRequest_BasicOffer(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	offer := buildBolt12Offer(t, nodeKey, 50000)

	ir, payerKey, err := BuildInvoiceRequest(offer)
	require.NoError(t, err)
	require.NotNil(t, payerKey)

	// Verify offer fields are mirrored.
	var desc []byte
	ir.OfferDescription.WhenSome(
		func(r tlv.RecordT[tlv.TlvType10, tlv.Blob]) {
			desc = r.Val
		},
	)
	require.Equal(t, "test offer", string(desc))

	require.Equal(
		t, uint64(50000),
		getUint64Field(ir.OfferAmount),
	)

	// Verify payer fields are set.
	require.True(t, hasOptField(ir.InvreqPayerID))
	require.True(t, hasOptField(ir.InvreqMetadata))

	// Verify the payer ID matches the returned key.
	var payerID []byte
	ir.InvreqPayerID.WhenSome(
		func(r tlv.RecordT[tlv.TlvType88, *btcec.PublicKey]) {
			payerID = r.Val.SerializeCompressed()
		},
	)
	require.Equal(
		t, payerKey.PubKey().SerializeCompressed(), payerID,
	)

	// Verify signature is present and valid.
	require.True(t, hasOptField(ir.Signature))

	// Re-encode and decode to verify round-trip, then check sig.
	irBytes, err := ir.Encode()
	require.NoError(t, err)

	decoded, err := bolt12.DecodeInvoiceRequest(irBytes)
	require.NoError(t, err)

	// The reader ends with the signature check against invreq_payer_id.
	require.NoError(t, bolt12.ValidateInvoiceRequestRead(
		decoded, testChainHash(), bolt12.Bolt12Features,
	))
}

// TestBuildInvoiceRequest_WithAmount verifies that invreq_amount is set when
// the offer has no fixed amount.
func TestBuildInvoiceRequest_WithAmount(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	offer := buildBolt12Offer(t, nodeKey, 0) // No fixed amount.

	ir, _, err := BuildInvoiceRequest(
		offer, WithAmount(25000),
	)
	require.NoError(t, err)

	require.Equal(
		t, uint64(25000),
		getUint64Field(ir.InvreqAmount),
	)
}

// TestBuildInvoiceRequest_WithQuantity verifies that invreq_quantity is set
// when provided.
func TestBuildInvoiceRequest_WithQuantity(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	offer := buildBolt12Offer(t, nodeKey, 1000)

	// invreq_quantity is only valid when the offer advertises
	// offer_quantity_max, so set one before requesting a quantity.
	offer.OfferQuantityMax = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType20, bolt12.TUint64]{Val: bolt12.TUint64(5)},
	)

	ir, _, err := BuildInvoiceRequest(
		offer, WithQuantity(3),
	)
	require.NoError(t, err)

	require.Equal(
		t, uint64(3),
		getUint64Field(ir.InvreqQuantity),
	)
}

// TestBuildInvoiceRequest_WithPayerNote verifies that invreq_payer_note is set
// when provided.
func TestBuildInvoiceRequest_WithPayerNote(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	offer := buildBolt12Offer(t, nodeKey, 1000)

	ir, _, err := BuildInvoiceRequest(
		offer, WithPayerNote("for coffee"),
	)
	require.NoError(t, err)

	var note []byte
	ir.InvreqPayerNote.WhenSome(
		func(r tlv.RecordT[tlv.TlvType89, tlv.Blob]) {
			note = r.Val
		},
	)
	require.Equal(t, "for coffee", string(note))
}

// TestBuildSingleHopReplyPath verifies the reply path has one hop and the
// introduction node matches the given pubkey.
func TestBuildSingleHopReplyPath(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)

	pathInfo, err := BuildSingleHopReplyPath(nodeKey.PubKey())
	require.NoError(t, err)
	require.NotNil(t, pathInfo)
	require.NotNil(t, pathInfo.Path)
	require.Len(t, pathInfo.Path.BlindedHops, 1)
	require.Equal(
		t, nodeKey.PubKey(),
		pathInfo.Path.IntroductionPoint,
	)
}

// TestValidateInvoiceReply pins the combined payer-side invoice validation:
// it passes for a correctly signed invoice in both the cleartext
// (offer_issuer_id) and blinded (offer_paths) modes, and rejects an invoice
// signed by the wrong node in each.
func TestValidateInvoiceReply(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	otherKey := testKey2(t)

	// The blinded offer carries a genuine route-blinding path, so its
	// blinded_node_id differs from the signing node's identity key. A
	// fixture that reused the identity key would pass even when the
	// receiver signs under the wrong identity.
	blindedOffer, blindedNodeID, pathKey := buildBlindedOffer(
		t, nodeKey, 50000,
	)
	require.False(t, blindedNodeID.IsEqual(nodeKey.PubKey()),
		"fixture must blind the node id away from the identity key")

	impersonated, _, impersonatedPathKey := buildBlindedOffer(
		t, nodeKey, 50000,
	)

	tests := []struct {
		name      string
		offer     *bolt12.Offer
		signer    NodeSigner
		pathKey   *btcec.PublicKey
		finalNode *btcec.PublicKey
		wantErr   error
	}{
		{
			name:   "valid cleartext",
			offer:  buildBolt12Offer(t, nodeKey, 50000),
			signer: NewPrivKeySigner(nodeKey),
			// offer_issuer_id present, so no hop pubkey is needed.
		},
		{
			name:      "valid blinded",
			offer:     blindedOffer,
			signer:    NewPrivKeySigner(nodeKey),
			pathKey:   pathKey,
			finalNode: blindedNodeID,
		},
		{
			name:      "blinded impersonation",
			offer:     impersonated,
			signer:    NewPrivKeySigner(nodeKey),
			pathKey:   impersonatedPathKey,
			finalNode: otherKey.PubKey(),
			wantErr:   bolt12.ErrUnexpectedInvoiceNodeID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ir, _, err := BuildInvoiceRequest(tc.offer)
			require.NoError(t, err)

			irBytes, err := ir.Encode()
			require.NoError(t, err)
			ir, err = bolt12.DecodeInvoiceRequest(irBytes)
			require.NoError(t, err)

			result, err := GenerateInvoice(
				ir, tc.signer, nil, [32]byte{}, tc.pathKey,
			)
			require.NoError(t, err)

			invBytes, err := result.Invoice.Encode()
			require.NoError(t, err)
			inv, err := bolt12.DecodeInvoice(invBytes)
			require.NoError(t, err)

			err = ValidateInvoiceReply(
				inv, ir, tc.finalNode, testChainHash(),
				time.Now(),
			)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

// buildBlindedOffer creates a bolt12.Offer that advertises a real blinded
// path (offer_paths) with no offer_issuer_id, so the payer must bind the
// received invoice to the final blinded node. It returns the offer, that
// path's final blinded_node_id, and the ephemeral path key the receiver would
// see on the incoming request.
func buildBlindedOffer(t *testing.T, key *btcec.PrivateKey,
	amountMsat uint64) (*bolt12.Offer, *btcec.PublicKey,
	*btcec.PublicKey) {

	t.Helper()

	// A single-hop path to the receiver: it is entered with the session
	// key, so that key is also the path key the receiver sees.
	sessionKey := testSeededKey(t, 200)

	path, err := sphinx.BuildBlindedPath(sessionKey, []*sphinx.HopInfo{{
		NodePub:   key.PubKey(),
		PlainText: []byte{0},
	}})
	require.NoError(t, err)

	introPub, err := lnwire.NewPubkeyIntro(path.Path.IntroductionPoint)
	require.NoError(t, err)

	hops := make([]lnwire.BlindedHop, len(path.Path.BlindedHops))
	for i, hop := range path.Path.BlindedHops {
		hops[i] = lnwire.BlindedHop{
			BlindedNodeID: hop.BlindedNodePub,
			EncryptedData: hop.CipherText,
		}
	}

	offer := &bolt12.Offer{
		OfferPaths: tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType16, lnwire.BlindedPaths]{
				Val: lnwire.BlindedPaths{
					Paths: []lnwire.BlindedPath{{
						IntroductionNode: introPub,
						BlindingPoint: path.Path.
							BlindingPoint,
						Hops: hops,
					}},
				},
			},
		),
		OfferDescription: tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType10, tlv.Blob]{
				Val: []byte("blinded offer"),
			},
		),
	}

	if amountMsat > 0 {
		amt := bolt12.TUint64(amountMsat)
		offer.OfferAmount = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType8, bolt12.TUint64]{
				Val: amt,
			},
		)
	}

	finalHop := hops[len(hops)-1]

	return offer, finalHop.BlindedNodeID, sessionKey.PubKey()
}

// buildBolt12Offer creates a bolt12.Offer for testing.
func buildBolt12Offer(t *testing.T, key *btcec.PrivateKey,
	amountMsat uint64) *bolt12.Offer {

	t.Helper()

	offer := &bolt12.Offer{}

	offer.OfferIssuerID = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType22, *btcec.PublicKey]{
			Val: key.PubKey(),
		},
	)
	offer.OfferDescription = tlv.SomeRecordT(
		tlv.RecordT[tlv.TlvType10, tlv.Blob]{
			Val: []byte("test offer"),
		},
	)

	if amountMsat > 0 {
		amt := bolt12.TUint64(amountMsat)
		offer.OfferAmount = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType8, bolt12.TUint64]{
				Val: amt,
			},
		)
	}

	return offer
}

// testKey2 returns a second deterministic test key distinct from testKey.
func testKey2(t *testing.T) *btcec.PrivateKey {
	t.Helper()

	var seed [32]byte
	for i := range seed {
		seed[i] = byte(i + 100)
	}

	key, _ := btcec.PrivKeyFromBytes(seed[:])

	return key
}
