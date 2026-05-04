package bolt12

// The charset constant and the toBech32Bytes / toBech32Chars helpers below are
// a deliberate copy of the unexported alphabet primitives in
// btcsuite/btcd/btcutil/bech32. The upstream public API
// (btcutil/bech32.Encode / Decode) always wraps the BIP-173 BCH checksum,
// while BOLT 12 omits the checksum because BIP-340 signatures over the
// Merkle root already secure the payload — so the bolt12 envelope has to
// reach below the public API to share the alphabet layer. Removing this
// duplication requires a btcutil change that exports either the alphabet
// primitives or a small AlphabetCodec; see zettel
// 202605011415-bech32-alphabet-helpers-belong-in-btcutil for the durable plan.

import (
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcutil/bech32"
)

const (
	// HRPOffer is the human-readable prefix for BOLT 12 offers.
	HRPOffer = "lno"

	// HRPInvoiceRequest is the human-readable prefix for BOLT 12 invoice
	// requests.
	HRPInvoiceRequest = "lnr"

	// HRPInvoice is the human-readable prefix for BOLT 12 invoices.
	HRPInvoice = "lni"

	// charset is the set of valid bech32 characters.
	charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

	// maxBolt12StringLen caps the size of a BOLT 12 bech32 input
	// before any allocation. Beyond ~150K characters there is no
	// legitimate carrier (URL, QR code, NFC) and the limit prevents
	// hostile inputs from forcing large allocations during decoding.
	maxBolt12StringLen = 150_000
)

// validHRPs lists the only HRPs the BOLT 12 codec emits. Encode rejects
// anything else so callers cannot accidentally produce strings that
// will fail to round-trip through Decode's HRP whitelist.
var validHRPs = map[string]struct{}{
	HRPOffer:          {},
	HRPInvoiceRequest: {},
	HRPInvoice:        {},
}

// Decode parses a BOLT 12 bech32-encoded string and returns the
// human-readable prefix and the decoded data bytes. Unlike standard bech32,
// BOLT 12 strings have no checksum. The '+' character may appear between
// bech32 characters as a continuation marker, optionally followed by
// whitespace.
func Decode(s string) (string, []byte, error) {
	if len(s) > maxBolt12StringLen {
		return "", nil, fmt.Errorf("bolt12: input length %d "+
			"exceeds limit %d", len(s), maxBolt12StringLen)
	}

	cleaned, err := stripContinuation(s)
	if err != nil {
		return "", nil, err
	}

	if len(cleaned) == 0 {
		return "", nil, fmt.Errorf("bolt12: empty string")
	}

	// The characters must be either all lowercase or all uppercase.
	lower := strings.ToLower(cleaned)
	upper := strings.ToUpper(cleaned)
	if cleaned != lower && cleaned != upper {
		return "", nil, fmt.Errorf("bolt12: string not all " +
			"lowercase or all uppercase")
	}

	cleaned = lower

	// Find the separator. The last '1' separates the HRP from data.
	one := strings.LastIndexByte(cleaned, '1')
	if one < 1 || one+1 >= len(cleaned) {
		return "", nil, fmt.Errorf("bolt12: missing or invalid " +
			"separator")
	}

	hrp := cleaned[:one]
	dataStr := cleaned[one+1:]

	// Validate and convert each character to its bech32 value.
	data5bit, err := toBech32Bytes(dataStr)
	if err != nil {
		return "", nil, fmt.Errorf("bolt12: invalid data: %w", err)
	}

	// Convert from base32 (5-bit groups) to base256 (8-bit bytes).
	data8bit, err := bech32.ConvertBits(data5bit, 5, 8, false)
	if err != nil {
		return "", nil, fmt.Errorf("bolt12: base conversion "+
			"failed: %w", err)
	}

	return hrp, data8bit, nil
}

// Encode serializes data bytes into a BOLT 12 bech32-encoded string with
// the given human-readable prefix. No checksum is appended. The HRP is
// validated against the BOLT 12 whitelist (lno/lnr/lni) so callers can
// only emit strings that Decode will accept.
func Encode(hrp string, data []byte) (string, error) {
	hrp = strings.ToLower(hrp)
	if _, ok := validHRPs[hrp]; !ok {
		return "", fmt.Errorf("bolt12: unsupported HRP %q "+
			"(want lno/lnr/lni)", hrp)
	}

	// Convert from base256 to base32.
	data5bit, err := bech32.ConvertBits(data, 8, 5, true)
	if err != nil {
		return "", fmt.Errorf("bolt12: base conversion "+
			"failed: %w", err)
	}

	chars, err := toBech32Chars(data5bit)
	if err != nil {
		return "", fmt.Errorf("bolt12: char conversion "+
			"failed: %w", err)
	}

	return hrp + "1" + chars, nil
}

// stripContinuation removes '+' continuation characters and any trailing
// whitespace that follows them from a BOLT 12 encoded string. The '+'
// character must appear between two bech32 characters — it cannot be at
// the start, end, or adjacent to another '+'.
func stripContinuation(s string) (string, error) {
	if len(s) == 0 {
		return "", nil
	}

	var b strings.Builder
	b.Grow(len(s))

	i := 0
	for i < len(s) {
		ch := s[i]

		if ch == '+' {
			// '+' must not be at the start.
			if b.Len() == 0 {
				return "", fmt.Errorf("bolt12: '+' at " +
					"start of string")
			}

			// Skip the '+' and any following whitespace.
			i++
			for i < len(s) && isWhitespace(s[i]) {
				i++
			}

			// After skipping whitespace, we must land on a bech32
			// character (not end-of-string, not another '+').
			if i >= len(s) {
				return "", fmt.Errorf("bolt12: '+' at " +
					"end of string")
			}
			if s[i] == '+' {
				return "", fmt.Errorf("bolt12: consecutive " +
					"'+' characters")
			}

			continue
		}

		b.WriteByte(ch)
		i++
	}

	return b.String(), nil
}

// isWhitespace returns true for space, tab, newline, and carriage return.
func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// toBech32Bytes converts a string of bech32 characters to their 5-bit
// integer values.
func toBech32Bytes(s string) ([]byte, error) {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		idx := strings.IndexByte(charset, s[i])
		if idx < 0 {
			return nil, fmt.Errorf("invalid character '%c' at "+
				"position %d", s[i], i)
		}
		result[i] = byte(idx)
	}

	return result, nil
}

// toBech32Chars converts 5-bit values to their bech32 character
// representation.
func toBech32Chars(data []byte) (string, error) {
	result := make([]byte, len(data))
	for i, b := range data {
		if int(b) >= len(charset) {
			return "", fmt.Errorf("invalid data byte: %d", b)
		}
		result[i] = charset[b]
	}

	return string(result), nil
}
