package bolt12

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/tlv"
)

// ErrEmptyMerkleInput is returned by MerkleRoot when the input contains
// no TLVs. The previous behaviour silently returned the all-zero hash,
// which any forged-but-empty message would also produce — a verifier
// could then accept signatures over an unrelated empty digest.
var ErrEmptyMerkleInput = errors.New(
	"cannot compute Merkle root over empty TLV set",
)

// taggedHash computes SHA256(SHA256(tag) || SHA256(tag) || msg) per the
// BIP-340 tagged hash convention.
func taggedHash(tag string, msg []byte) [32]byte {
	return *chainhash.TaggedHash([]byte(tag), msg)
}

// leafHash computes H("LnLeaf", fullTLVBytes) for a single TLV field.
func leafHash(fullTLVBytes []byte) [32]byte {
	return taggedHash("LnLeaf", fullTLVBytes)
}

// nonceHash computes H("LnNonce" || firstTLV, tlvTypeBigSize) for a
// single TLV field. The tag includes the raw bytes of the first TLV in
// the stream. The message is the BigSize-encoded type of the current
// TLV field.
//
// The tag concatenation uses string(firstTLV) deliberately — Go's
// []byte → string conversion is a byte-faithful copy and the spec
// defines the tag as the literal byte concatenation, not a UTF-8
// joining. A "cleanup" refactor that swaps to a builder or fmt.Sprintf
// risks turning non-UTF-8 bytes into U+FFFD replacements and breaking
// signature compatibility against every other BOLT 12 implementation.
func nonceHash(firstTLV []byte, tlvType tlv.Type) [32]byte {
	tag := "LnNonce" + string(firstTLV)

	var buf [8]byte
	var typeBuf bytes.Buffer
	// WriteVarInt only fails on a Writer error; bytes.Buffer.Write
	// is documented to never return one, so the discard is safe.
	_ = tlv.WriteVarInt(&typeBuf, uint64(tlvType), &buf)

	return taggedHash(tag, typeBuf.Bytes())
}

// branchHash computes H("LnBranch", lesser || greater) where the two
// child hashes are sorted lexicographically with the lesser hash first.
func branchHash(a, b [32]byte) [32]byte {
	if bytes.Compare(a[:], b[:]) > 0 {
		a, b = b, a
	}

	var msg [64]byte
	copy(msg[:32], a[:])
	copy(msg[32:], b[:])

	return taggedHash("LnBranch", msg[:])
}

// MerkleRoot computes the Merkle root of the given TLV records. Each
// record is encoded in isolation via its TLV stream form to derive the
// per-leaf full type+length+value bytes that feed both the LnLeaf and
// LnNonce digests. The records must be in canonical order (ascending by
// type, no duplicates); typically the caller passes the output of a
// PureTLVMessage's AllRecords filtered through signableTLVs.
//
// An empty input returns ErrEmptyMerkleInput; signing or verifying
// over an empty stream is never the intended behaviour and would
// otherwise collide with the all-zero digest.
func MerkleRoot(records []tlv.Record) ([32]byte, error) {
	if len(records) == 0 {
		return [32]byte{}, ErrEmptyMerkleInput
	}

	// Encode each record on its own to recover the same per-field
	// type+length+value bytes the original wire stream contained.
	// The spec's nonce tag binds to the bytes of the first TLV, so
	// the per-record encoding must match what the producer signed.
	encoded := make([][]byte, len(records))
	for i := range records {
		r := records[i]
		buf, err := lnwire.EncodeRecords([]tlv.Record{r})
		if err != nil {
			return [32]byte{}, fmt.Errorf(
				"encode record %d (type %d): %w",
				i, r.Type(), err,
			)
		}
		encoded[i] = buf
	}

	firstTLV := encoded[0]

	branches := make([][32]byte, len(records))
	for i, r := range records {
		leaf := leafHash(encoded[i])
		nonce := nonceHash(firstTLV, r.Type())
		branches[i] = branchHash(leaf, nonce)
	}

	// Combine branches pairwise until a single root remains.
	for len(branches) > 1 {
		var next [][32]byte
		for i := 0; i < len(branches); i += 2 {
			if i+1 < len(branches) {
				combined := branchHash(
					branches[i], branches[i+1],
				)
				next = append(next, combined)
			} else {
				// Odd element is promoted unchanged.
				next = append(next, branches[i])
			}
		}
		branches = next
	}

	return branches[0], nil
}
