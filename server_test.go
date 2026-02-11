package lnd

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/connmgr"
	"github.com/lightningnetwork/lnd/lncfg"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/peer"
	"github.com/stretchr/testify/require"
)

// mockConnMgr implements the connMgr interface for testing. It tracks all
// Connect and Remove calls without making real network connections.
type mockConnMgr struct {
	mu           sync.Mutex
	connectCalls []*connmgr.ConnReq
	removeCalls  []uint64
}

func (m *mockConnMgr) Connect(c *connmgr.ConnReq) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectCalls = append(m.connectCalls, c)
}

func (m *mockConnMgr) Remove(id uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeCalls = append(m.removeCalls, id)
}

func (m *mockConnMgr) Start() {}
func (m *mockConnMgr) Stop()  {}

func (m *mockConnMgr) getConnectCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.connectCalls)
}

func (m *mockConnMgr) getRemoveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.removeCalls)
}

// newTestServer creates a minimal server instance with only the fields needed
// for persistent connection management tests.
func newTestServer(t *testing.T, cm connMgr) *server {
	t.Helper()

	s := &server{
		cfg: &Config{
			MinBackoff: time.Second,
			Dev:        &lncfg.DevConfig{},
		},
		connMgr:                 cm,
		persistentPeers:         make(map[string]bool),
		persistentPeersBackoff:  make(map[string]time.Duration),
		persistentConnReqs:      make(map[string][]*connmgr.ConnReq),
		persistentPeerAddrs:     make(map[string][]*lnwire.NetAddress),
		persistentRetryCancels:  make(map[string]chan struct{}),
		peersByPub:              make(map[string]*peer.Brontide),
		inboundPeers:            make(map[string]*peer.Brontide),
		outboundPeers:           make(map[string]*peer.Brontide),
		ignorePeerTermination:   make(map[*peer.Brontide]struct{}),
		scheduledPeerConnection: make(map[string]func()),
		quit:                    make(chan struct{}),
	}

	return s
}

// generateTestPubKey creates a new random public key for testing.
func generateTestPubKey(t *testing.T) *btcec.PublicKey {
	t.Helper()
	priv, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	return priv.PubKey()
}

// TestNodeAnnouncementTimestampComparison tests the timestamp comparison
// logic used in setSelfNode to ensure node announcements have strictly
// increasing timestamps at second precision (as required by BOLT-07 and
// enforced by the database storage).
func TestNodeAnnouncementTimestampComparison(t *testing.T) {
	t.Parallel()

	// Use a simple base time for the tests.
	baseTime := int64(1000)

	tests := []struct {
		name              string
		srcNodeLastUpdate time.Time
		nodeLastUpdate    time.Time
		expectedResult    time.Time
		description       string
	}{
		{
			name:              "same second different nanoseconds",
			srcNodeLastUpdate: time.Unix(baseTime, 0),
			nodeLastUpdate:    time.Unix(baseTime, 500_000_000),
			expectedResult:    time.Unix(baseTime+1, 0),
			description: "Edge case: timestamps in same second " +
				"but different nanoseconds. Must increment " +
				"to avoid persisting same second-level " +
				"timestamp.",
		},
		{
			name:              "different seconds",
			srcNodeLastUpdate: time.Unix(baseTime, 0),
			nodeLastUpdate:    time.Unix(baseTime+2, 0),
			expectedResult:    time.Unix(baseTime+2, 0),
			description: "Normal case: current time is already " +
				"in a different (later) second. No increment " +
				"needed.",
		},
		{
			name:              "exactly equal",
			srcNodeLastUpdate: time.Unix(baseTime, 123456789),
			nodeLastUpdate:    time.Unix(baseTime, 123456789),
			expectedResult:    time.Unix(baseTime+1, 123456789),
			description: "Timestamps are identical. Must " +
				"increment to ensure strictly greater " +
				"timestamp.",
		},
		{
			name:              "exactly equal - zero nanoseconds",
			srcNodeLastUpdate: time.Unix(baseTime, 0),
			nodeLastUpdate:    time.Unix(baseTime, 0),
			expectedResult:    time.Unix(baseTime+1, 0),
			description: "Timestamps are identical at second " +
				"precision (0 nanoseconds), as would be read " +
				"from DB. Must increment.",
		},
		{
			name:              "clock skew - persisted is newer",
			srcNodeLastUpdate: time.Unix(baseTime+5, 0),
			nodeLastUpdate:    time.Unix(baseTime+3, 0),
			expectedResult:    time.Unix(baseTime+6, 0),
			description: "Clock went backwards: persisted " +
				"timestamp is newer than current time. Must " +
				"increment from persisted timestamp.",
		},
		{
			name:              "clock skew - same second",
			srcNodeLastUpdate: time.Unix(baseTime+5, 100_000_000),
			nodeLastUpdate:    time.Unix(baseTime+5, 900_000_000),
			expectedResult:    time.Unix(baseTime+6, 100_000_000),
			description: "Clock skew within same second. Must " +
				"increment to ensure strictly greater " +
				"second-level timestamp.",
		},
		{
			name: "same second component different " +
				"minute",
			srcNodeLastUpdate: time.Unix(baseTime, 0),
			nodeLastUpdate:    time.Unix(baseTime+60, 0),
			expectedResult:    time.Unix(baseTime+60, 0),
			description: "Same seconds component (:00) but " +
				"different minutes. Current time is later. " +
				"Verifies we use .Unix() not .Second().",
		},
		{
			name: "lower second component but " +
				"later time",
			srcNodeLastUpdate: time.Unix(baseTime+58, 0),
			nodeLastUpdate:    time.Unix(baseTime+63, 0),
			expectedResult:    time.Unix(baseTime+63, 0),
			description: "Persisted has second=58, current has " +
				"second=3 (next minute). Current is later " +
				"overall. Verifies .Unix() not .Second().",
		},
		{
			name: "higher second component but " +
				"earlier time",
			srcNodeLastUpdate: time.Unix(baseTime+63, 0),
			nodeLastUpdate:    time.Unix(baseTime+58, 0),
			expectedResult:    time.Unix(baseTime+64, 0),
			description: "Persisted has second=3 (next minute), " +
				"current has second=58. Persisted is later " +
				"overall. Verifies .Unix() not .Second().",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := calculateNodeAnnouncementTimestamp(
				tc.srcNodeLastUpdate,
				tc.nodeLastUpdate,
			)

			// Verify we got the expected result.
			require.Equal(
				t, tc.expectedResult, result,
				"Unexpected result: %s", tc.description,
			)

			// Verify result is strictly greater than persisted
			// timestamp. This is an additional check to ensure
			// the result is strictly greater than the persisted
			// timestamp.
			require.Greater(
				t, result.Unix(), tc.srcNodeLastUpdate.Unix(),
				"Result must be strictly greater than "+
					"persisted timestamp: %s",
				tc.description,
			)
		})
	}
}

// TestConnectToPeerAccumulation reproduces Bug 1: repeated ConnectToPeer calls
// with perm=true accumulate ConnReqs without canceling existing ones.
func TestConnectToPeerAccumulation(t *testing.T) {
	t.Parallel()

	cm := &mockConnMgr{}
	s := newTestServer(t, cm)

	pubKey := generateTestPubKey(t)
	addr := &lnwire.NetAddress{
		IdentityKey: pubKey,
		Address:     &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 9735},
	}

	targetPub := string(pubKey.SerializeCompressed())

	// Pre-populate persistent peer state so ConnectToPeer doesn't fail
	// early.
	s.persistentPeers[targetPub] = true
	s.persistentPeersBackoff[targetPub] = time.Second

	// Call ConnectToPeer 10 times with perm=true. Each call should cancel
	// existing ConnReqs before creating a new one.
	for i := 0; i < 10; i++ {
		err := s.ConnectToPeer(addr, true, time.Second)
		require.NoError(t, err)
	}

	s.mu.Lock()
	count := len(s.persistentConnReqs[targetPub])
	s.mu.Unlock()

	// BUG: Without fix, count is 10 (one per call, no dedup).
	// AFTER FIX: count should be 1 (cancel + replace each time).
	require.Equal(t, 1, count,
		"expected 1 ConnReq after fix, got %d (accumulation bug)",
		count)
}
