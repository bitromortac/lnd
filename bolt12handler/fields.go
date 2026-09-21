package bolt12handler

import (
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/tlv"
)

// hasOptField returns true if the optional record is set.
func hasOptField[T tlv.TlvType, V any](opt tlv.OptionalRecordT[T, V]) bool {
	set := false
	opt.WhenSome(func(_ tlv.RecordT[T, V]) {
		set = true
	})

	return set
}

// getUint64Field extracts the uint64 value from an optional TUint64 record,
// returning 0 if absent.
func getUint64Field[T tlv.TlvType](opt tlv.OptionalRecordT[T,
	bolt12.TUint64]) uint64 {

	var val uint64
	opt.WhenSome(
		func(r tlv.RecordT[T, bolt12.TUint64]) {
			val = uint64(r.Val)
		},
	)

	return val
}
