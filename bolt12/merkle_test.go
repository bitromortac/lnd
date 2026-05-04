package bolt12

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// sigTestVector represents a test case from signature-test.json.
type sigTestVector struct {
	Comment  string            `json:"comment"`
	TLV      string            `json:"tlv"`
	Bolt12   string            `json:"bolt12"`
	FirstTLV string            `json:"first-tlv"` //nolint:tagliatelle // BOLT 12 spec vector key.
	Leaves   []json.RawMessage `json:"leaves"`
	Branches []json.RawMessage `json:"branches"`
	Merkle   string            `json:"merkle"`

	SignatureTag string `json:"signature_tag"`
	Signature    string `json:"signature"`
}

// TestMerkleRootVectors verifies the Merkle root computation against
// every test case in signature-test.json.
func TestMerkleRootVectors(t *testing.T) {
	t.Parallel()

	vectors := loadSignatureVectors(t)
	require.NotEmpty(t, vectors)

	for _, tc := range vectors {
		t.Run(tc.Comment, func(t *testing.T) {
			t.Parallel()

			var records []tlv.Record

			switch {
			case tc.Bolt12 != "":
				// Decode the bech32 string to get TLV
				// bytes, then convert into the record
				// view the new MerkleRoot consumes.
				_, tlvBytes, err := Decode(tc.Bolt12)
				require.NoError(t, err)

				records = streamToRecords(t, tlvBytes)

			case tc.TLV == "n1":
				// Build records from the leaf
				// descriptions. The n1 namespace is
				// synthetic — there is no bech32
				// representation, so we recover each
				// record from its hex prefix.
				records = buildN1Records(t, tc)

			default:
				t.Fatalf("vector %q: neither bolt12 nor "+
					"n1 — refusing to assume the "+
					"wrong synthesis path", tc.Comment)
			}

			// Filter out signature fields (type 240) which
			// are excluded from the Merkle tree.
			var filtered []tlv.Record
			for _, r := range records {
				if r.Type() < 240 {
					filtered = append(filtered, r)
				}
			}

			root, err := MerkleRoot(filtered)
			require.NoError(t, err)

			expectedRoot, err := hex.DecodeString(tc.Merkle)
			require.NoError(t, err)
			require.Equal(
				t, expectedRoot, root[:],
				"merkle root mismatch",
			)
		})
	}
}

// buildN1Records constructs tlv.Record entries for the simple n1 test
// vectors by parsing the leaf hex values from the test JSON.
func buildN1Records(t *testing.T, tc sigTestVector) []tlv.Record {
	t.Helper()

	var result []tlv.Record

	for _, leafJSON := range tc.Leaves {
		var leafMap map[string]string
		require.NoError(t, json.Unmarshal(leafJSON, &leafMap))

		// Find the LnLeaf key to extract the TLV bytes.
		// Key format: H(`LnLeaf`,<hex>)
		prefix := "H(`LnLeaf`,"
		for key := range leafMap {
			if len(key) > len(prefix) &&
				key[:len(prefix)] == prefix {

				// Extract hex between the comma and
				// closing paren.
				hexStr := key[len(prefix) : len(key)-1]
				fullBytes, err := hex.DecodeString(hexStr)
				require.NoError(t, err)

				result = append(
					result, recordFromBytes(t, fullBytes),
				)

				break
			}
		}
	}

	return result
}

// TestLeafHash verifies individual leaf hash computations from the test
// vectors.
func TestLeafHash(t *testing.T) {
	t.Parallel()

	// From the first test vector: H("LnLeaf", 010203e8)
	input, _ := hex.DecodeString("010203e8")
	got := leafHash(input)
	expected, _ := hex.DecodeString(
		"67a2a995433890d8fe0c18a1765ad19e98f1fcfeff14c13a45bb" +
			"c80964a78cf7",
	)
	require.Equal(t, expected, got[:])
}

// TestNonceHash verifies the nonce hash computation.
func TestNonceHash(t *testing.T) {
	t.Parallel()

	firstTLV, _ := hex.DecodeString("010203e8")

	// Type 1 nonce
	got := nonceHash(firstTLV, 1)
	expected, _ := hex.DecodeString(
		"255a95f5b6b3c6997e2838dc4d9348807fb6da8eb7bbc02d3066" +
			"2d144718b6aa",
	)
	require.Equal(t, expected, got[:])

	// Type 2 nonce
	got2 := nonceHash(firstTLV, 2)
	expected2, _ := hex.DecodeString(
		"12bc15565410d8e3251a6fb1c53a2d360f39a9f65afb8403ef87" +
			"5016e34ff678",
	)
	require.Equal(t, expected2, got2[:])
}

// TestBranchHash verifies the branch hash computation.
func TestBranchHash(t *testing.T) {
	t.Parallel()

	// From test vector 2: combining tlv1+nonce and tlv2+nonce branches.
	a, _ := hex.DecodeString(
		"19d6ecfa3be88d29c30e56167f58526d7695dfac9cb95e1256de" +
			"b222c92db4d0",
	)
	b, _ := hex.DecodeString(
		"b013756c8fee86503a0b4abdab4cddeb1af5d344ca6fc2fa8b6c" +
			"08938caa6f93",
	)

	var aArr, bArr [32]byte
	copy(aArr[:], a)
	copy(bArr[:], b)

	got := branchHash(aArr, bArr)
	expected, _ := hex.DecodeString(
		"c3774abbf4815aa54ccaa026bff6581f01f3be5fe814c620a252" +
			"534f434bc0d1",
	)
	require.Equal(t, expected, got[:])
}

// TestMerkleVectorIntermediateHashes asserts every named LnLeaf,
// LnNonce, and LnBranch entry from each signature-test.json vector
// matches the hash our implementation produces. Without this, the
// test suite only pinned the final root, leaving the 14 intermediate
// hashes for vectors 1, 2, 3 unverified — a regression in any
// individual hash function would still produce a correct root
// fortuitously and pass the existing test.
func TestMerkleVectorIntermediateHashes(t *testing.T) {
	t.Parallel()

	for _, tc := range loadSignatureVectors(t) {
		// The n1 vectors are synthesised; pubkey-bearing
		// invoice_request leaves are recoverable from the
		// bech32 string. In both cases the leaf hex appears in
		// the JSON `H('LnLeaf', <hex>)` keys, so we walk those
		// directly.
		t.Run(tc.Comment, func(t *testing.T) {
			t.Parallel()

			firstTLV, err := hex.DecodeString(tc.FirstTLV)
			require.NoError(t, err)

			for i, leafJSON := range tc.Leaves {
				assertLeafEntry(t, leafJSON, firstTLV, i)
			}

			// Branch entries each carry exactly one
			// H('LnBranch', <hashA||hashB>) key.
			for i, branchJSON := range tc.Branches {
				assertBranchEntry(t, branchJSON, i)
			}
		})
	}
}

// assertLeafEntry validates the three named hashes inside a single
// vector leaf object: the LnLeaf hash over the TLV bytes, the LnNonce
// hash bound to the first TLV plus a per-leaf type identifier, and
// the LnBranch hash combining the two.
func assertLeafEntry(
	t *testing.T, leafJSON json.RawMessage, firstTLV []byte, idx int,
) {
	t.Helper()

	var entries map[string]string
	require.NoError(t, json.Unmarshal(leafJSON, &entries))

	const (
		leafPrefix   = "H(`LnLeaf`,"
		noncePrefix  = "H(`LnNonce`|first-tlv,"
		branchPrefix = "H(`LnBranch`,"
	)

	var (
		leafKey, leafExpected     string
		nonceKey, nonceExpected   string
		branchKey, branchExpected string
	)
	for k, v := range entries {
		switch {
		case len(k) > len(leafPrefix) &&
			k[:len(leafPrefix)] == leafPrefix:
			leafKey, leafExpected = k, v
		case len(k) > len(noncePrefix) &&
			k[:len(noncePrefix)] == noncePrefix:
			nonceKey, nonceExpected = k, v
		case len(k) > len(branchPrefix) &&
			k[:len(branchPrefix)] == branchPrefix:
			branchKey, branchExpected = k, v
		}
	}
	require.NotEmpty(t, leafKey,
		"leaf %d: missing LnLeaf key", idx)
	require.NotEmpty(t, nonceKey,
		"leaf %d: missing LnNonce key", idx)
	require.NotEmpty(t, branchKey,
		"leaf %d: missing LnBranch key", idx)

	leafHex := leafKey[len(leafPrefix) : len(leafKey)-1]
	leafBytes, err := hex.DecodeString(leafHex)
	require.NoError(t, err)

	gotLeaf := leafHash(leafBytes)
	wantLeaf, err := hex.DecodeString(leafExpected)
	require.NoError(t, err)
	require.Equal(t, wantLeaf, gotLeaf[:],
		"leaf %d: LnLeaf hash mismatch", idx)

	// The nonce key encodes a per-leaf type identifier as the
	// final segment after the comma. For older vectors the
	// segment is the type name ("tlv1-type"); newer ones use a
	// raw type number ("1"). We extract the leaf's leading TLV
	// type from its hex prefix and use that — the spec says the
	// nonce binds to the first TLV plus the leaf's own type.
	leafType := leafTypeFromHex(t, leafBytes)
	gotNonce := nonceHash(firstTLV, leafType)
	wantNonce, err := hex.DecodeString(nonceExpected)
	require.NoError(t, err)
	require.Equal(t, wantNonce, gotNonce[:],
		"leaf %d: LnNonce hash mismatch", idx)

	gotBranch := branchHash(gotLeaf, gotNonce)
	wantBranch, err := hex.DecodeString(branchExpected)
	require.NoError(t, err)
	require.Equal(t, wantBranch, gotBranch[:],
		"leaf %d: LnBranch hash mismatch", idx)
}

// assertBranchEntry validates the branch hash for one entry in the
// vector's `branches` array. Each entry's H('LnBranch', <hashA||hashB>)
// key carries the two child hashes concatenated; the value is the
// expected combined hash.
func assertBranchEntry(
	t *testing.T, branchJSON json.RawMessage, idx int,
) {
	t.Helper()

	var entries map[string]string
	require.NoError(t, json.Unmarshal(branchJSON, &entries))

	const branchPrefix = "H(`LnBranch`,"

	var key, expected string
	for k, v := range entries {
		if len(k) > len(branchPrefix) &&
			k[:len(branchPrefix)] == branchPrefix {

			key, expected = k, v
		}
	}
	require.NotEmpty(t, key, "branch %d: missing LnBranch key", idx)

	hexConcat := key[len(branchPrefix) : len(key)-1]
	concat, err := hex.DecodeString(hexConcat)
	require.NoError(t, err)
	require.Equal(t, 64, len(concat),
		"branch %d: expected 64 bytes of child hashes", idx)

	var a, b [32]byte
	copy(a[:], concat[:32])
	copy(b[:], concat[32:])

	got := branchHash(a, b)
	want, err := hex.DecodeString(expected)
	require.NoError(t, err)
	require.Equal(t, want, got[:],
		"branch %d: LnBranch hash mismatch", idx)
}

// leafTypeFromHex parses the leading varint of a TLV-encoded leaf to
// recover its type number. The signature-test.json LnNonce entries
// bind the nonce to this type, so we must reproduce the parse here
// to compute the same nonce hash.
func leafTypeFromHex(t *testing.T, leafBytes []byte) tlv.Type {
	t.Helper()

	var buf [8]byte
	r := newByteReader(leafBytes)
	typ, err := tlv.ReadVarInt(r, &buf)
	require.NoError(t, err)

	return tlv.Type(typ)
}

// byteReaderWrapper wraps a byte slice for use with ReadVarInt.
type byteReaderWrapper struct {
	data []byte
	pos  int
}

func newByteReader(data []byte) *byteReaderWrapper {
	return &byteReaderWrapper{data: data}
}

func (r *byteReaderWrapper) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, nil
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n

	return n, nil
}

// TestPropertyMerkleOrderSensitivity asserts that for any non-trivial
// raw TLV sequence the Merkle root depends on the input order. The
// receiver-to-sender invoice flow signs a tree built over a
// type-sorted stream; if the root were order-insensitive, an
// attacker could permute fields without invalidating the signature.
func TestPropertyMerkleOrderSensitivity(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		// Need at least two leaves with distinct types — types
		// tagged into the nonce hash, so identical types would
		// produce identical leaves and a swap would be a no-op.
		n := rapid.IntRange(2, 8).Draw(t, "leafCount")
		records := make([]tlv.Record, n)
		for i := 0; i < n; i++ {
			records[i] = recordFromTestBytes(
				t, tlv.Type(i+1), drawTLVValue(t),
			)
		}

		root1, err := MerkleRoot(records)
		require.NoError(t, err)

		// Swap the first two entries.
		swapped := make([]tlv.Record, len(records))
		copy(swapped, records)
		swapped[0], swapped[1] = swapped[1], swapped[0]

		root2, err := MerkleRoot(swapped)
		require.NoError(t, err)

		require.NotEqual(t, root1, root2,
			"swapping two distinct leaves did not change root")
	})
}

// recordFromTestBytes builds a tlv.Record whose value is the given
// payload. This is the rapid-compatible analogue of recordFromBytes
// for tests that synthesise individual TLV values.
func recordFromTestBytes(t *rapid.T, typ tlv.Type, value []byte) tlv.Record {
	v := value

	return tlv.MakePrimitiveRecord(typ, &v)
}

// drawTLVValue synthesises the value-side payload for a single TLV
// record. Used by the Merkle order-sensitivity property to build
// leaves that the hash functions can ingest; the type is bound
// separately in recordFromTestBytes.
func drawTLVValue(t *rapid.T) []byte {
	payloadLen := rapid.IntRange(1, 8).Draw(t, "payloadLen")

	return rapid.SliceOfN(
		rapid.Byte(), payloadLen, payloadLen,
	).Draw(t, "payload")
}

// TestMerkleRootEmptyInput pins the contract for an empty leaf set:
// MerkleRoot returns ErrEmptyMerkleInput, never the all-zero digest.
// The all-zero hash is a valid SHA-256 output that could collide with
// a legitimately computed root, so a verifier accepting it could be
// tricked by a forged-but-empty message. This is a regression guard
// against the historical behaviour where MerkleRoot([]) silently
// returned [32]byte{}.
func TestMerkleRootEmptyInput(t *testing.T) {
	t.Parallel()

	t.Run("nil slice", func(t *testing.T) {
		t.Parallel()

		root, err := MerkleRoot(nil)
		require.ErrorIs(t, err, ErrEmptyMerkleInput)
		require.Equal(t, [32]byte{}, root)
	})

	t.Run("empty slice", func(t *testing.T) {
		t.Parallel()

		root, err := MerkleRoot([]tlv.Record{})
		require.ErrorIs(t, err, ErrEmptyMerkleInput)
		require.Equal(t, [32]byte{}, root)
	})
}
