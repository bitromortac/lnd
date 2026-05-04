package bolt12

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// TestSignatureVerifyVector verifies the signature from the
// invoice_request test vector in signature-test.json.
func TestSignatureVerifyVector(t *testing.T) {
	t.Parallel()

	vectors := loadSignatureVectors(t)

	// Find the invoice_request test vector.
	var tc sigTestVector
	for _, v := range vectors {
		if v.Bolt12 != "" && v.TLV == "invoice_request" {
			tc = v
			break
		}
	}
	require.NotEmpty(t, tc.Bolt12)

	// Decode the bech32 string and convert into the record view
	// the new MerkleRoot consumes.
	_, tlvBytes, err := Decode(tc.Bolt12)
	require.NoError(t, err)

	records := streamToRecords(t, tlvBytes)

	// Filter out signature fields (type >= 240).
	var unsigned []tlv.Record
	for _, r := range records {
		if r.Type() < 240 {
			unsigned = append(unsigned, r)
		}
	}

	// Compute merkle root.
	root, err := MerkleRoot(unsigned)
	require.NoError(t, err)
	expectedRoot, err := hex.DecodeString(tc.Merkle)
	require.NoError(t, err)
	require.Equal(t, expectedRoot, root[:])

	// Verify the tagged hash used for signing.
	require.Equal(t, "lightninginvoice_requestsignature",
		tc.SignatureTag)
	sigDigest := taggedHash(tc.SignatureTag, root[:])

	// The expected digest is stored under a JSON key with a comma
	// which can't be parsed via struct tags. Parse it manually.
	rawVectors := loadSignatureRawVectors(t)

	var rawMap map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rawVectors[3], &rawMap))

	var expectedDigestHex string
	require.NoError(t, json.Unmarshal(
		rawMap["H(signature_tag,merkle)"], &expectedDigestHex,
	))

	expectedDigest, err := hex.DecodeString(expectedDigestHex)
	require.NoError(t, err)
	require.Equal(t, expectedDigest, sigDigest[:])

	// Verify the signature.
	sigBytes, err := hex.DecodeString(tc.Signature)
	require.NoError(t, err)

	var sig [64]byte
	copy(sig[:], sigBytes)

	bobPrivKey, bobPubKey := bobKey()

	err = VerifySignature(
		"invoice_request", "signature",
		root, sig, bobPubKey,
	)
	require.NoError(t, err)

	// Sign with the same key and verify round-trip.
	newSig, err := SignMessage(
		"invoice_request", "signature",
		root, bobPrivKey,
	)
	require.NoError(t, err)

	err = VerifySignature(
		"invoice_request", "signature",
		root, newSig, bobPubKey,
	)
	require.NoError(t, err)
}

// TestSignatureVerifyAllVectors drives every signature-test.json
// entry that carries both a bolt12 string and a signature through the
// full verify pipeline. The original TestSignatureVerifyVector pinned
// only the invoice_request case; vectors 1, 2, and 3 were left
// unchecked.
func TestSignatureVerifyAllVectors(t *testing.T) {
	t.Parallel()

	bobPriv, bobPub := bobKey()
	require.Equal(t, signatureTagPrefix,
		"lightning",
		"signatureTagPrefix divergence breaks tagged hashing",
	)

	covered := 0
	for i, tc := range loadSignatureVectors(t) {
		if tc.Bolt12 == "" || tc.Signature == "" {
			continue
		}

		t.Run(tc.Comment, func(t *testing.T) {
			t.Parallel()

			_, tlvBytes, err := Decode(tc.Bolt12)
			require.NoError(t, err)

			records := streamToRecords(t, tlvBytes)

			root, err := MerkleRoot(signableTLVs(records))
			require.NoError(t, err)

			expectedRoot, err := hex.DecodeString(tc.Merkle)
			require.NoError(t, err)
			require.Equal(t, expectedRoot, root[:],
				"vector %d merkle root mismatch", i)

			sigBytes, err := hex.DecodeString(tc.Signature)
			require.NoError(t, err)
			var sig [64]byte
			copy(sig[:], sigBytes)

			tag := tc.SignatureTag
			require.True(t,
				len(tag) > len(signatureTagPrefix),
				"vector tag too short: %q", tag,
			)
			messageName, fieldName := splitTag(t, tag)

			require.NoError(t, VerifySignature(
				messageName, fieldName,
				root, sig, bobPub,
			))

			// Round-trip with bobPriv so SignMessage and
			// VerifySignature compose without external help.
			fresh, err := SignMessage(
				messageName, fieldName, root, bobPriv,
			)
			require.NoError(t, err)
			require.NoError(t, VerifySignature(
				messageName, fieldName, root, fresh, bobPub,
			))
		})
		covered++
	}

	require.GreaterOrEqual(t, covered, 1,
		"expected at least one signed vector",
	)
}

// splitTag extracts (messageName, fieldName) from a "lightning" ||
// messageName || fieldName composite tag. fieldName is always
// "signature" in the spec; everything between the prefix and the
// trailing "signature" is the message name.
func splitTag(t *testing.T, tag string) (string, string) {
	t.Helper()

	const fieldName = "signature"
	require.True(t, len(tag) > len(signatureTagPrefix)+len(fieldName),
		"tag too short to split: %q", tag,
	)

	require.Equal(t, signatureTagPrefix,
		tag[:len(signatureTagPrefix)],
		"tag missing 'lightning' prefix: %q", tag,
	)
	require.Equal(t, fieldName,
		tag[len(tag)-len(fieldName):],
		"tag missing 'signature' suffix: %q", tag,
	)

	return tag[len(signatureTagPrefix) : len(tag)-len(fieldName)],
		fieldName
}

// TestSignatureVerifyRejectsTampering walks the four classes of
// tampering a malicious mediator can attempt: a bit flip in the root,
// in the signature bytes, swap to a different verifying key, and
// replay under a different message tag. Each must fail verification
// — silent acceptance of any of them lets the tree-of-fields
// guarantee collapse.
func TestSignatureVerifyRejectsTampering(t *testing.T) {
	t.Parallel()

	bobPriv, bobPub := bobKey()

	var msg [32]byte
	for i := range msg {
		msg[i] = byte(i + 1)
	}
	sig, err := SignMessage("invoice_request", "signature", msg, bobPriv)
	require.NoError(t, err)

	// Sanity: untouched signature still verifies.
	require.NoError(t, VerifySignature(
		"invoice_request", "signature", msg, sig, bobPub,
	))

	t.Run("tampered root", func(t *testing.T) {
		t.Parallel()

		tampered := msg
		tampered[0] ^= 0x01
		require.ErrorIs(t,
			VerifySignature(
				"invoice_request", "signature",
				tampered, sig, bobPub,
			),
			ErrInvalidSignature,
		)
	})

	t.Run("tampered signature byte", func(t *testing.T) {
		t.Parallel()

		tamperedSig := sig
		tamperedSig[0] ^= 0xff
		err := VerifySignature(
			"invoice_request", "signature",
			msg, tamperedSig, bobPub,
		)
		require.Error(t, err)
	})

	t.Run("wrong public key", func(t *testing.T) {
		t.Parallel()

		_, alicePub := aliceKey()
		require.ErrorIs(t,
			VerifySignature(
				"invoice_request", "signature",
				msg, sig, alicePub,
			),
			ErrInvalidSignature,
		)
	})

	t.Run("cross-tag replay rejected", func(t *testing.T) {
		t.Parallel()

		// Same root, same signature, but verify under the
		// invoice tag instead of invoice_request.
		require.ErrorIs(t,
			VerifySignature(
				"invoice", "signature",
				msg, sig, bobPub,
			),
			ErrInvalidSignature,
		)
	})

	t.Run("malformed 64-byte signature", func(t *testing.T) {
		t.Parallel()

		var malformed [64]byte
		err := VerifySignature(
			"invoice_request", "signature",
			msg, malformed, bobPub,
		)
		require.Error(t, err)
	})
}

// TestVerifyInvoiceDirect drives VerifyInvoice end to end using a
// minimal valid Invoice constructed via validInvoice. The original
// suite only reached VerifyInvoice through the invoice_request
// pipeline, leaving the missing-node-id and invalid-pubkey error
// branches untested.
func TestVerifyInvoiceDirect(t *testing.T) {
	t.Parallel()

	priv, pub := bobKey()

	t.Run("valid round-trip verifies", func(t *testing.T) {
		t.Parallel()

		inv := validInvoice(t)
		inv.InvoiceNodeID = tlv.SomeRecordT(
			tlv.NewPrimitiveRecord[tlv.TlvType176](pub),
		)
		_, err := inv.Encode()
		require.NoError(t, err)

		sig, err := SignInvoice(inv, priv)
		require.NoError(t, err)
		inv.Signature = tlv.SomeRecordT(
			tlv.NewPrimitiveRecord[tlv.TlvType240, [64]byte](sig),
		)

		require.NoError(t, VerifyInvoice(inv))
	})

	t.Run("missing invoice_node_id", func(t *testing.T) {
		t.Parallel()

		inv := validInvoice(t)
		inv.InvoiceNodeID = tlv.OptionalRecordT[
			tlv.TlvType176, *btcec.PublicKey,
		]{}

		err := VerifyInvoice(inv)
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing invoice_node_id")
	})

	t.Run("invalid invoice_node_id", func(t *testing.T) {
		t.Parallel()

		inv := validInvoice(t)
		// A present-but-nil invoice_node_id passes the presence
		// check but has no key to verify against.
		inv.InvoiceNodeID = tlv.SomeRecordT(
			tlv.NewPrimitiveRecord[tlv.TlvType176](
				(*btcec.PublicKey)(nil),
			),
		)

		err := VerifyInvoice(inv)
		require.ErrorIs(t, err, ErrInvalidPubKey)
		require.Contains(t, err.Error(), "invoice_node_id")
	})

	t.Run("missing signature", func(t *testing.T) {
		t.Parallel()

		inv := validInvoice(t)
		inv.InvoiceNodeID = tlv.SomeRecordT(
			tlv.NewPrimitiveRecord[tlv.TlvType176](pub),
		)

		err := VerifyInvoice(inv)
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing signature")
	})
}

// TestSignableTLVsFilteringBoundaries pins the inclusion rule for the
// Merkle input. The spec excludes types in [240, 1000]; everything
// outside that range contributes. Drift here would either include
// type 240 (the signature itself, breaking commit-to-tree-root
// semantics) or exclude experimental types > 1000 (silently dropping
// fields the writer expected to commit to).
func TestSignableTLVsFilteringBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		typ      tlv.Type
		included bool
	}{
		{typ: 0, included: true},
		{typ: 239, included: true},
		{typ: 240, included: false},
		{typ: 500, included: false},
		{typ: 1000, included: false},
		{typ: 1001, included: true},
		{typ: 1_000_000_000, included: true},
	}

	records := make([]tlv.Record, 0, len(tests))
	for _, tc := range tests {
		// An empty value blob is enough — the filter only inspects
		// each record's Type.
		var v []byte
		records = append(
			records, tlv.MakePrimitiveRecord(tc.typ, &v),
		)
	}

	got := signableTLVs(records)
	gotTypes := make(map[tlv.Type]bool, len(got))
	for _, r := range got {
		gotTypes[r.Type()] = true
	}

	for _, tc := range tests {
		require.Equal(t, tc.included, gotTypes[tc.typ],
			"type %d inclusion mismatch", tc.typ)
	}
}

