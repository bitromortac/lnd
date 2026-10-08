package lnd

import (
	"testing"

	"github.com/lightningnetwork/lnd/lncfg"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/stretchr/testify/require"
)

// bolt12MigrationsInBuild reports whether this build applies every BOLT 12
// development migration.
func bolt12MigrationsInBuild() bool {
	known := make(map[string]struct{})
	for _, migration := range sqldb.GetMigrations() {
		known[migration.Name] = struct{}{}
	}

	for _, name := range bolt12Migrations {
		if _, ok := known[name]; !ok {
			return false
		}
	}

	return true
}

// TestValidateBolt12Offers verifies that the startup check refuses BOLT 12
// offers on a node that cannot hold their tables or deliver their messages,
// and accepts them only in a build with the development migrations.
func TestValidateBolt12Offers(t *testing.T) {
	t.Parallel()

	newConfig := func() *Config {
		return &Config{
			ProtocolOptions: &lncfg.ProtocolOptions{},
			DB:              &lncfg.DB{UseNativeSQL: true},
		}
	}

	cfg := newConfig()
	cfg.ProtocolOptions.NoOnionMessagesOption = true
	require.ErrorContains(t, validateBolt12Offers(cfg), "onion messages")

	cfg = newConfig()
	cfg.ProtocolOptions.NoRouteBlindingOption = true
	require.ErrorContains(t, validateBolt12Offers(cfg), "route blinding")

	cfg = newConfig()
	cfg.DB.UseNativeSQL = false
	require.ErrorContains(
		t, validateBolt12Offers(cfg), "db.use-native-sql",
	)

	// A release build has no BOLT 12 migrations and must refuse the
	// flag. A build with the development migrations accepts it.
	err := validateBolt12Offers(newConfig())
	if bolt12MigrationsInBuild() {
		require.NoError(t, err)
	} else {
		require.ErrorContains(t, err, "not part of this build")
	}
}
