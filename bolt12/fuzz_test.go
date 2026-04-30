package bolt12

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/tlv"
)

// seedOffersVectors enriches a fuzzer's corpus with the bolt12 strings
// from offers-test.json. Every spec-compliant string becomes an entry
// the fuzzer's mutator can branch from, which dramatically improves
// the chance of hitting code paths that randomly drawn bytes never
// reach. Errors are not fatal — a missing fixture leaves the fuzzer
// with only its hand-coded seeds rather than aborting the whole run.
func seedOffersVectors(f *testing.F) {
	f.Helper()

	data, err := os.ReadFile("test-vectors/offers-test.json")
	if err != nil {
		return
	}

	var vectors []offersTestVector
	if err := json.Unmarshal(data, &vectors); err != nil {
		return
	}

	for _, v := range vectors {
		if v.Bolt12 == "" {
			continue
		}
		f.Add(v.Bolt12)
	}
}

// seedSignatureVectors mirrors seedOffersVectors for signature-test.json.
// The bolt12 strings there exercise the invoice_request format; the
// raw TLV streams could be reconstructed from the leaf hex but adding
// them at the bytes level is easier via the bolt12 strings the
// decoder accepts directly.
func seedSignatureVectors(f *testing.F) {
	f.Helper()

	data, err := os.ReadFile("test-vectors/signature-test.json")
	if err != nil {
		return
	}

	var vectors []sigTestVector
	if err := json.Unmarshal(data, &vectors); err != nil {
		return
	}

	for _, v := range vectors {
		if v.Bolt12 != "" {
			f.Add(v.Bolt12)
		}
	}
}

// seedTLVStreamsFromOffers populates the byte-level fuzzers' corpus
// with TLV streams extracted from each offers-test.json bolt12 string.
// This gives the byte-level decoders the same coverage benefit
// without forcing them through the bech32 path first.
func seedTLVStreamsFromOffers(f *testing.F) {
	f.Helper()

	data, err := os.ReadFile("test-vectors/offers-test.json")
	if err != nil {
		return
	}

	var vectors []offersTestVector
	if err := json.Unmarshal(data, &vectors); err != nil {
		return
	}

	for _, v := range vectors {
		if v.Bolt12 == "" {
			continue
		}
		_, tlvBytes, decErr := Decode(v.Bolt12)
		if decErr != nil {
			continue
		}
		f.Add(tlvBytes)
	}
}

// FuzzDecodeOffer exercises the offer decoder with arbitrary TLV
// byte input. The decoder must return cleanly (success or error)
// without panicking; on success the returned offer must round-trip
// — re-encoding the decoded struct and decoding-then-re-encoding
// must yield byte-identical output. A regression that drops or
// duplicates a TLV record on the encode path would otherwise only
// surface at the wire boundary.
func FuzzDecodeOffer(f *testing.F) {
	// Seed: minimal valid offer with offer_issuer_id only.
	f.Add([]byte{
		0x16, 0x21, // type=22, length=33
		0x02, 0xee, 0xc7, 0x24, 0x5d, 0x6b, 0x7d, 0x2c,
		0xcb, 0x30, 0x38, 0x0b, 0xfb, 0xe2, 0xa3, 0x64,
		0x8c, 0xd7, 0xa9, 0x42, 0x65, 0x3f, 0x5a, 0xa3,
		0x40, 0xed, 0xce, 0xa1, 0xf2, 0x83, 0x68, 0x66,
		0x19,
	})
	seedTLVStreamsFromOffers(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		offer, err := decodeOffer(data)
		if err != nil {
			return
		}
		if offer == nil {
			t.Fatal("nil offer with nil error")
		}

		encoded, err := offer.Encode()
		if err != nil {
			// A successfully decoded offer that fails the
			// writer-side validate is allowed (read accepts
			// constraints write rejects). Skip round-trip.
			return
		}

		again, err := decodeOffer(encoded)
		if err != nil {
			t.Fatalf("round-trip decode failed: %v", err)
		}
		encoded2, err := again.Encode()
		if err != nil {
			t.Fatalf("second encode failed: %v", err)
		}
		if !bytes.Equal(encoded, encoded2) {
			t.Fatal("round-trip changed encoded bytes")
		}
	})
}

// FuzzDecodeInvoiceRequest mirrors FuzzDecodeOffer for
// invoice_request bytes. Beyond no-panic, a successfully decoded
// request must round-trip through Encode → Decode → Encode without
// altering the encoded bytes.
func FuzzDecodeInvoiceRequest(f *testing.F) {
	f.Add([]byte{
		0x00, 0x08, // type=0 invreq_metadata, length=8
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		ir, err := DecodeInvoiceRequest(data)
		if err != nil {
			return
		}
		if ir == nil {
			t.Fatal("nil invoice request with nil error")
		}

		encoded, err := ir.Encode()
		if err != nil {
			return
		}

		again, err := DecodeInvoiceRequest(encoded)
		if err != nil {
			t.Fatalf("round-trip decode failed: %v", err)
		}
		encoded2, err := again.Encode()
		if err != nil {
			t.Fatalf("second encode failed: %v", err)
		}
		if !bytes.Equal(encoded, encoded2) {
			t.Fatal("round-trip changed encoded bytes")
		}
	})
}

// FuzzDecodeInvoice mirrors FuzzDecodeOffer for invoice bytes,
// including the same Encode → Decode → Encode round-trip property.
func FuzzDecodeInvoice(f *testing.F) {
	f.Add([]byte{
		0xfd, 0x00, 0xa8, 0x20, // type=168 (varint), length=32
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		inv, err := DecodeInvoice(data)
		if err != nil {
			return
		}
		if inv == nil {
			t.Fatal("nil invoice with nil error")
		}

		encoded, err := inv.Encode()
		if err != nil {
			return
		}

		again, err := DecodeInvoice(encoded)
		if err != nil {
			t.Fatalf("round-trip decode failed: %v", err)
		}
		encoded2, err := again.Encode()
		if err != nil {
			t.Fatalf("second encode failed: %v", err)
		}
		if !bytes.Equal(encoded, encoded2) {
			t.Fatal("round-trip changed encoded bytes")
		}
	})
}

// FuzzDecodeInvoiceError exercises the invoice_error decoder. The
// round-trip property is at the byte level: a valid decode must
// re-encode and re-decode-and-re-encode to identical bytes. A
// regression that drops a record on encode would surface as a
// length mismatch on the second encode.
func FuzzDecodeInvoiceError(f *testing.F) {
	f.Add([]byte{
		0x05, 0x05, // type=5 error, length=5
		'h', 'e', 'l', 'l', 'o',
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		ie, err := DecodeInvoiceError(data)
		if err != nil {
			return
		}
		if ie == nil {
			t.Fatal("nil invoice_error with nil error")
		}

		encoded, err := ie.Encode()
		if err != nil {
			return
		}

		again, err := DecodeInvoiceError(encoded)
		if err != nil {
			t.Fatalf("round-trip decode failed: %v", err)
		}
		encoded2, err := again.Encode()
		if err != nil {
			t.Fatalf("second encode failed: %v", err)
		}
		if !bytes.Equal(encoded, encoded2) {
			t.Fatal("round-trip changed encoded bytes")
		}
	})
}

// FuzzDecodeOfferString exercises the bech32 + TLV decoding path.
// Seeded with every valid offers-test.json bolt12 string so every
// spec-conformant prefix is a starting point for the mutator.
func FuzzDecodeOfferString(f *testing.F) {
	f.Add("lno1zcss9mk8y3wkklfvevcrszlmu23kfrxh49p" +
		"x20665dqwmn4p72pksese")
	seedOffersVectors(f)

	f.Fuzz(func(t *testing.T, s string) {
		_, _ = DecodeOfferString(
			s, farFutureNow(), bitcoinMainnetGenesisHash,
		)
	})
}

// FuzzDecodeInvoiceRequestString exercises the lnr bech32 path.
// Mirrors FuzzDecodeOfferString for invoice request strings; the
// signature-test.json invoice_request vector is the natural seed.
func FuzzDecodeInvoiceRequestString(f *testing.F) {
	seedSignatureVectors(f)

	f.Fuzz(func(t *testing.T, s string) {
		_, _ = DecodeInvoiceRequestString(
			s, bitcoinMainnetGenesisHash,
		)
	})
}

// FuzzBech32RoundTrip pins the Encode/Decode bijection on the bech32
// layer alone, decoupled from any TLV-level concerns. The mutator
// can permute HRP, length, and bytes; any input that round-trips
// successfully must yield the original (hrp, data) pair.
func FuzzBech32RoundTrip(f *testing.F) {
	f.Add(uint8(0), []byte{0x00})
	f.Add(uint8(1), []byte{0xab, 0xcd, 0xef})
	f.Add(uint8(2), bytes.Repeat([]byte{0x42}, 256))

	hrps := []string{HRPOffer, HRPInvoiceRequest, HRPInvoice}

	f.Fuzz(func(t *testing.T, hrpIdx uint8, data []byte) {
		if len(data) == 0 {
			return
		}

		hrp := hrps[int(hrpIdx)%len(hrps)]
		encoded, err := Encode(hrp, data)
		if err != nil {
			return
		}

		gotHRP, gotData, err := Decode(encoded)
		if err != nil {
			t.Fatalf(
				"decode after successful encode "+
					"failed: %v", err,
			)
		}
		if gotHRP != hrp {
			t.Fatalf(
				"hrp mismatch: encoded with %q, "+
					"decoded as %q", hrp, gotHRP,
			)
		}
		if !bytes.Equal(gotData, data) {
			t.Fatalf(
				"data mismatch: input %s, output %s",
				hex.EncodeToString(data),
				hex.EncodeToString(gotData),
			)
		}
	})
}

// FuzzMerkleRootDeterminism asserts MerkleRoot is a pure function of
// its inputs. Computing the root twice on the same record set must
// produce identical bytes. A regression that introduced
// non-determinism (e.g. a map iteration order leaking into the hash
// input) would silently break signature reproducibility.
func FuzzMerkleRootDeterminism(f *testing.F) {
	seedTLVStreamsFromOffers(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		stream, err := tlv.NewStream()
		if err != nil {
			t.Fatalf("new stream: %v", err)
		}

		typeMap, err := stream.DecodeWithParsedTypesP2P(
			bytes.NewReader(data),
		)
		if err != nil || len(typeMap) == 0 {
			return
		}

		records := lnwire.TlvMapToRecords(typeMap)

		root1, err := MerkleRoot(records)
		if err != nil {
			return
		}

		root2, err := MerkleRoot(records)
		if err != nil {
			t.Fatalf("second MerkleRoot failed: %v", err)
		}

		if root1 != root2 {
			t.Fatal("MerkleRoot produced different roots " +
				"for identical input")
		}
	})
}
