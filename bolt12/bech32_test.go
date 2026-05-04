package bolt12

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// formatStringTestVector represents a single test case from the BOLT 12
// format-string-test.json file.
type formatStringTestVector struct {
	Comment string `json:"comment"`
	Valid   bool   `json:"valid"`
	String  string `json:"string"`
}

// TestBech32FormatStringVectors runs through every test case in the spec's
// format-string-test.json to verify our bech32 encoder/decoder handles
// continuations, case, and edge cases correctly.
func TestBech32FormatStringVectors(t *testing.T) {
	t.Parallel()

	vectors := loadFormatStringVectors(t)
	require.NotEmpty(t, vectors)

	for _, tc := range vectors {
		t.Run(tc.Comment, func(t *testing.T) {
			t.Parallel()

			hrp, decoded, err := Decode(tc.String)

			if !tc.Valid {
				require.Error(t, err, "expected error for: %s",
					tc.Comment)

				return
			}

			require.NoError(t, err, "unexpected error for: %s",
				tc.Comment)
			require.Equal(t, HRPOffer, hrp)
			require.NotEmpty(t, decoded)

			// Round-trip: re-encode and decode again.
			encoded, err := Encode(hrp, decoded)
			require.NoError(t, err)

			hrp2, decoded2, err := Decode(encoded)
			require.NoError(t, err)
			require.Equal(t, hrp, hrp2)
			require.Equal(t, decoded, decoded2)
		})
	}
}

// TestBech32RoundTrip verifies that encoding then decoding returns the
// original data for each supported HRP.
func TestBech32RoundTrip(t *testing.T) {
	t.Parallel()

	testData := []byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03}

	for _, hrp := range []string{HRPOffer, HRPInvoiceRequest, HRPInvoice} {
		t.Run(hrp, func(t *testing.T) {
			t.Parallel()

			encoded, err := Encode(hrp, testData)
			require.NoError(t, err)
			require.True(t, len(encoded) > len(hrp)+1)

			gotHRP, gotData, err := Decode(encoded)
			require.NoError(t, err)
			require.Equal(t, hrp, gotHRP)
			require.Equal(t, testData, gotData)
		})
	}
}

// TestBech32DecodeErrors verifies that various malformed inputs produce
// errors.
func TestBech32DecodeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "no separator",
			input: "lnoabcdef",
		},
		{
			name:  "separator only",
			input: "1",
		},
		{
			name:  "no data after separator",
			input: "lno1",
		},
		{
			name:  "invalid character",
			input: "lno1b",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Decode(tc.input)
			require.Error(t, err)
		})
	}
}

// TestStripContinuation verifies the '+' stripping logic in isolation.
func TestStripContinuation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "no continuation",
			input: "lno1abc",
			want:  "lno1abc",
		},
		{
			name:  "simple continuation",
			input: "lno1a+bc",
			want:  "lno1abc",
		},
		{
			name:  "continuation with whitespace",
			input: "lno1a+ bc",
			want:  "lno1abc",
		},
		{
			name:  "continuation with newline",
			input: "lno1a+\nbc",
			want:  "lno1abc",
		},
		{
			name:  "continuation with crlf and space",
			input: "lno1a+\r\n bc",
			want:  "lno1abc",
		},
		{
			name:    "trailing plus",
			input:   "lno1abc+",
			wantErr: true,
		},
		{
			name:    "trailing plus with space",
			input:   "lno1abc+ ",
			wantErr: true,
		},
		{
			name:    "leading plus",
			input:   "+lno1abc",
			wantErr: true,
		},
		{
			name:    "consecutive plus",
			input:   "lno1a++bc",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := stripContinuation(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestEncodeRejectsUnknownHRP asserts that the BOLT 12 whitelist of
// HRPs is enforced on encode so callers cannot emit strings that
// Decode would refuse on round-trip.
func TestEncodeRejectsUnknownHRP(t *testing.T) {
	t.Parallel()

	_, err := Encode("bogus", []byte{0x00})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported HRP")
}

// TestDecodeRejectsOversizeInput asserts the input length cap fires
// before any allocation.
func TestDecodeRejectsOversizeInput(t *testing.T) {
	t.Parallel()

	huge := strings.Repeat("a", maxBolt12StringLen+1)
	_, _, err := Decode(huge)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds limit")
}

// TestDecodeAcceptsUppercase pins the spec MUST that readers handle
// both all-lowercase and all-uppercase strings: a payload encoded
// lowercase, then ToUpper'd in transit (e.g. QR code), must decode
// back to the same HRP and bytes.
func TestDecodeAcceptsUppercase(t *testing.T) {
	t.Parallel()

	payload := []byte{0xde, 0xad, 0xbe, 0xef}
	encoded, err := Encode(HRPOffer, payload)
	require.NoError(t, err)

	uppered := strings.ToUpper(encoded)
	require.NotEqual(t, encoded, uppered)

	hrp, data, err := Decode(uppered)
	require.NoError(t, err)
	require.Equal(t, HRPOffer, hrp)
	require.Equal(t, payload, data)
}

// TestPropertyBech32RoundTrip asserts Encode and Decode form a
// bijection for arbitrary data payloads under each of the three
// BOLT 12 HRPs. The codec's correctness depends on this property; a
// hand-rolled table can only hit a small number of payload sizes,
// while rapid drives shrinking generators across the whole input
// space and minimises any counter-example it finds.
func TestPropertyBech32RoundTrip(t *testing.T) {
	t.Parallel()

	hrps := []string{HRPOffer, HRPInvoiceRequest, HRPInvoice}

	rapid.Check(t, func(t *rapid.T) {
		hrp := hrps[rapid.IntRange(0, len(hrps)-1).Draw(t, "hrp")]
		// Cap the payload at a length the codec actually
		// supports; rapid would otherwise fuzz unboundedly and
		// trip the maxBolt12StringLen guard which is the
		// codec's external contract, not what this property is
		// about.
		// Lower bound 1 because Decode requires the bech32 data
		// portion to be non-empty (a zero-length payload encodes
		// to just the HRP+separator and Decode rejects it as
		// missing the separator). The property is about the
		// codec's bijection on supported inputs.
		size := rapid.IntRange(1, 1024).Draw(t, "size")
		data := rapid.SliceOfN(
			rapid.Byte(), size, size,
		).Draw(t, "data")

		encoded, err := Encode(hrp, data)
		require.NoError(t, err)

		decodedHRP, decodedData, err := Decode(encoded)
		require.NoError(t, err)
		require.Equal(t, hrp, decodedHRP)
		require.Equal(t, data, decodedData)
	})
}
