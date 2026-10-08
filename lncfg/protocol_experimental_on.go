//go:build dev
// +build dev

package lncfg

// ExperimentalProtocol is a sub-config that houses any experimental protocol
// features that also require a build-tag to activate.
type ExperimentalProtocol struct {
	// Bolt12Offers turns on BOLT 12 offers: the offer store, the receiver
	// that answers invoice requests, and the offer payment RPCs. It needs
	// native SQL and a build with the BOLT 12 development migrations.
	Bolt12Offers bool `long:"bolt12-offers" description:"enable BOLT 12 offers (experimental); needs db.use-native-sql and a build with the BOLT 12 development migrations"`
}

// Bolt12OffersEnabled returns true if BOLT 12 offers are enabled.
func (p ExperimentalProtocol) Bolt12OffersEnabled() bool {
	return p.Bolt12Offers
}
