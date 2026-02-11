# Persistent ConnReq Accumulation — Detailed Analysis

## Problem Statement

Persistent connection requests (`ConnReq`) for offline peers accumulate unboundedly in the `server.persistentConnReqs` map. Users have reported 989+ ConnReqs for a single peer. The connection manager (`connmgr`) treats each ConnReq independently, retrying all of them with exponential backoff. This floods the connection manager, starves healthy peers of connection slots, and triggers automatic force closures — causing real financial loss.

## Architecture Background

### How persistent connections work

LND maintains long-lived connections to "persistent peers" — peers with open channels or peers explicitly added via `lncli connect --perm`. The lifecycle:

1. **Registration**: A peer becomes persistent via `ConnectToPeer(perm=true)` or by having open channels (detected at startup and in `peerTerminationWatcher`).

2. **Connection request creation**: A `connmgr.ConnReq` is created and passed to `connMgr.Connect()`. The ConnReq is also stored in `server.persistentConnReqs[pubkey]` so the server can cancel it later.

3. **Connection manager processing**: `connmgr` assigns the ConnReq an ID (atomically incrementing counter) and attempts the TCP+noise handshake. On failure, since `Permanent: true`, it retries with backoff.

4. **Success callback**: When a connection succeeds, `connmgr` calls `server.OutboundPeerConnected()`. The server cancels all other pending ConnReqs for that peer via `cancelConnReqs`.

5. **Disconnection**: When a peer disconnects, `peerTerminationWatcher` calls `connectToPersistentPeer` to re-establish the connection.

### Key data structures

```
server.persistentConnReqs    map[string][]*connmgr.ConnReq   // pubkey -> pending requests
server.persistentRetryCancels map[string]chan struct{}         // pubkey -> cancel channel for stagger goroutine
server.persistentPeers       map[string]bool                  // pubkey -> is persistent?
server.persistentPeerAddrs   map[string][]*lnwire.NetAddress  // pubkey -> known addresses
```

### The ConnReq ID lifecycle

A `ConnReq` starts with `ID() == 0` (UnassignedConnID). The ID is assigned atomically by `connmgr` when `Connect()` processes the request internally. There's a window between when the server creates the ConnReq and when connmgr assigns the ID — this window is central to Bug 3.

---

## Bug 1: `ConnectToPeer` accumulation

### Location
`server.go` — `func (s *server) ConnectToPeer(...)`, the `if perm` branch.

### Trigger
Any code path that calls `ConnectToPeer` with `perm=true` for a peer that already has pending ConnReqs. This includes:
- Repeated `lncli connect --perm <peer>` commands
- Autopilot requesting connections
- Any RPC client calling `ConnectPeer` repeatedly

### Root cause

```go
// BEFORE FIX (simplified):
if reqs, ok := s.persistentConnReqs[targetPub]; ok {
    srvrLog.Warnf("Already have %d persistent connection requests for %v, connecting anyway.", len(reqs), addr)
}
// Creates new ConnReq and APPENDS it — never cancels existing ones
s.persistentConnReqs[targetPub] = append(s.persistentConnReqs[targetPub], connReq)
```

The code logs a warning but takes no action. Each call appends a new ConnReq. The connection manager now manages N independent retry loops for the same peer.

### Reproduction
Call `ConnectToPeer(addr, true, timeout)` 10 times. Result: 10 ConnReqs in the map, 10 independent retry loops in connmgr.

### Fix
Call `cancelConnReqs(targetPub, nil)` before creating the new ConnReq. This removes all existing ConnReqs from both the map and the connection manager, then creates a single fresh one.

### Severity
**High**. This is the most direct accumulation path. Any user or automated system calling `connect --perm` periodically (common in node management scripts) triggers unbounded growth.

---

## Bug 2: `connectToPersistentPeer` goroutine race

### Location
`server.go` — `func (s *server) connectToPersistentPeer(pubKeyStr string)`, the cancel channel and goroutine spawn.

### Trigger
Rapid calls to `connectToPersistentPeer` for the same peer. This happens when:
- Multiple gossip updates arrive for the same peer in quick succession
- `peerTerminationWatcher` fires multiple times (e.g., rapid disconnect/reconnect cycles)
- Node announcement updates trigger address refreshes

### Root cause

```go
// BEFORE FIX (simplified):
cancelChan, ok := s.persistentRetryCancels[pubKeyStr]
if !ok {
    cancelChan = make(chan struct{})
    s.persistentRetryCancels[pubKeyStr] = cancelChan
}
// If cancelChan already exists, REUSE it — old goroutine keeps running

go func() {
    for _, addr := range addrMap {
        connReq := &connmgr.ConnReq{Addr: addr, Permanent: true}
        s.mu.Lock()
        s.persistentConnReqs[pubKeyStr] = append(s.persistentConnReqs[pubKeyStr], connReq)
        s.mu.Unlock()
        go s.connMgr.Connect(connReq)
        select {
        case <-cancelChan:  // only checked BETWEEN addresses
            return
        case <-ticker.C:
        }
    }
}()
```

Two problems:
1. **Cancel channel reuse**: If a cancel channel already exists, the code reuses it. The old goroutine is never signaled to stop. Both old and new goroutines run concurrently, both creating ConnReqs.

2. **Goroutine creates ConnReq before checking cancel**: Even if we close the cancel channel, the goroutine creates a ConnReq for its first address before reaching the `select` statement. For single-address peers (very common), the goroutine finishes its entire loop body before ever checking the cancel channel.

### Reproduction
Call `connectToPersistentPeer(pubkey)` 5 times for a peer with 1 address. Result: 5 goroutines each create 1 ConnReq = 5 ConnReqs.

### Fix
Two-part fix:
1. **Always close and replace the cancel channel**: `close(oldCancelChan)` before creating a new one. Old goroutines receive the close signal.
2. **Check cancel channel before appending**: Inside the goroutine, under `s.mu.Lock()`, do a non-blocking `select` on `cancelChan` before appending the ConnReq. Since the close happens under the same mutex in the parent function, and the goroutine checks under the same mutex, there's no race.

### Severity
**High**. This is the most frequent accumulation path in practice, since `connectToPersistentPeer` is called on every disconnect for every persistent peer.

---

## Bug 3: `cancelConnReqs` skips unassigned IDs

### Location
`server.go` — `func (s *server) cancelConnReqs(pubStr string, skip *uint64)`, the `UnassignedConnID` check.

### Trigger
`cancelConnReqs` is called while a ConnReq's `Connect()` goroutine hasn't been scheduled yet (or connmgr hasn't processed it yet). The ConnReq still has ID=0.

### Root cause

```go
// BEFORE FIX:
for _, connReq := range connReqs {
    connID := connReq.ID()
    if connID == UnassignedConnID {
        continue  // SKIP — can't call Remove(0)
    }
    s.connMgr.Remove(connID)
}
delete(s.persistentConnReqs, pubStr)  // map entry deleted regardless
```

The ConnReq is skipped because `Remove(0)` would be invalid. But the map entry is still deleted. Now:
- The server has no reference to this ConnReq
- When `connmgr` eventually processes the `Connect()` call, it assigns an ID and starts managing the ConnReq
- The ConnReq retries forever — the server can never cancel it because it lost the reference

This creates "ghost" ConnReqs — invisible to the server but actively managed by connmgr.

### Reproduction
Create a ConnReq, add it to the map, call `cancelConnReqs` before `Connect()` is processed. The ConnReq is orphaned.

### Fix
Instead of skipping, spawn a goroutine that polls `connReq.ID()` every 100ms (with a 10-second timeout). Once the ID is assigned, call `Remove(id)`. If the timeout expires (e.g., the Connect goroutine was also canceled), log a warning.

### Severity
**Medium**. This is a race condition that depends on timing. It's harder to trigger than Bugs 1 and 2, but each occurrence creates a permanently orphaned ConnReq. Over time, these accumulate.

### Open question
Is 10 seconds the right timeout? What if connmgr is under heavy load and takes longer to process? Should we use a different mechanism (e.g., connmgr providing a callback when the ID is assigned)? The current polling approach works but is not elegant. The TODO comment on `UnassignedConnID` suggests the right fix is to move ID generation into a method that returns an ID synchronously, but that requires changes to the `connmgr` package.

---

## Bug 4: `OutboundPeerConnected` partial cleanup on inbound collision

### Location
`server.go` — `func (s *server) OutboundPeerConnected(...)`, the `inboundPeers` check.

### Trigger
An outbound connection attempt succeeds (`OutboundPeerConnected` is called), but the peer already has an inbound connection. This happens when:
- Peer connects inbound while our outbound attempts are still pending
- Both sides try to connect simultaneously and inbound wins

### Root cause

```go
// BEFORE FIX:
if p, ok := s.inboundPeers[pubStr]; ok {
    if connReq != nil {
        s.connMgr.Remove(connReq.ID())  // Only removes THIS ConnReq
    }
    conn.Close()
    return
}
```

Only the specific ConnReq that triggered this callback is removed. If the peer has accumulated 5 ConnReqs (from Bugs 1/2), only 1 is removed. The other 4 continue retrying in connmgr. Each retry that succeeds hits this same path — removing 1 more — but new ConnReqs may accumulate faster than they're cleaned up.

### Contrast with the success path

Later in the same function, the success path does the right thing:

```go
if connReq != nil {
    ignore := connReq.ID()
    s.cancelConnReqs(pubStr, &ignore)  // Cancel ALL except the successful one
}
```

The inbound-exists path should do the same, but cancel ALL (no skip).

### Reproduction
Set up 5 pending ConnReqs for a peer, then simulate the inbound-exists path. Result: only 1 ConnReq removed, 4 remain.

### Fix
Replace `s.connMgr.Remove(connReq.ID())` with `s.cancelConnReqs(pubStr, nil)`. This removes all ConnReqs and retry channels. No skip is needed because we're discarding the outbound connection entirely.

### Severity
**Medium**. This is a cleanup failure that compounds with Bugs 1 and 2. On its own, it causes slow leaks. Combined with the other bugs, it means accumulated ConnReqs are never fully cleaned up even when the peer does connect.

---

## Interaction Between Bugs

The bugs compound each other:

```
Bug 1/2: ConnReqs accumulate (N per peer)
    |
    v
Bug 4: When peer connects inbound, only 1 of N is cleaned up
    |
    v
Bug 3: Some of the N have unassigned IDs and are silently skipped
    |
    v
Result: Ghost ConnReqs accumulate permanently
    |
    v
connmgr overloaded -> healthy peers dropped -> force closures
```

### Failure cascade

1. Offline peer accumulates 100+ ConnReqs (Bugs 1+2)
2. Peer briefly connects inbound, then disconnects
3. `OutboundPeerConnected` fires for one of the outbound attempts, sees inbound exists, removes 1 ConnReq (Bug 4 — 99 remain)
4. `cancelConnReqs` runs for the remaining 99, but some have unassigned IDs and are skipped (Bug 3)
5. Peer goes offline again, `connectToPersistentPeer` fires, adds more ConnReqs (Bug 2)
6. connmgr now managing 100+ ConnReqs for this single peer, each doing independent exponential backoff retries
7. Connection slots filled, healthy peers get dropped
8. Channels time out, HTLCs expire, force closures happen

---

## Defense-in-Depth: Safety Cap

Even with all 4 bugs fixed, a `maxPersistentConnReqsPerPeer = 10` cap is enforced before every append to `persistentConnReqs`. If the cap is hit (which shouldn't happen with the fixes), all existing ConnReqs are canceled before appending the new one. This prevents any future regression from causing unbounded growth.

The cap is enforced in two places:
- `ConnectToPeer` — before appending in the `perm` branch
- `connectToPersistentPeer` goroutine — before appending inside the stagger loop

---

## Areas for Further Investigation

### 1. connmgr internal queue
When `connMgr.Connect()` is called, the ConnReq enters an internal queue. If `Remove()` is called before the ConnReq is dequeued, does connmgr handle this correctly? The polling approach in Bug 3's fix assumes connmgr will eventually assign an ID, but what if the ConnReq is dropped from the queue?

### 2. Backoff state
`persistentPeersBackoff` tracks per-peer backoff durations. When ConnReqs are canceled and recreated, does the backoff reset? If so, the fixes might cause more aggressive reconnection attempts (good for fast recovery, but could be seen as more aggressive by the target peer).

### 3. Multi-address stagger behavior
`connectToPersistentPeer` staggers ConnReq creation across addresses by 10 seconds (`multiAddrConnectionStagger`). With the fix, closing the cancel channel causes old goroutines to exit mid-stagger. If a peer has 5 addresses and the function is called twice within 5 seconds, the first call only created ConnReqs for addresses 1-2 before being canceled. The second call recreates ConnReqs for all 5. The net effect is correct (one ConnReq per address), but there's a brief window where addresses 1-2 have no ConnReq (old one canceled, new one not yet created by the stagger goroutine). Is this acceptable?

### 4. Thread safety of ConnReq.ID()
`ConnReq.ID()` uses `atomic.LoadUint64`. The server reads it without holding a lock (in `cancelConnReqs`), while connmgr writes it from a different goroutine. The atomic operation makes this safe, but the polling loop in Bug 3's fix adds repeated atomic loads. Is there a more efficient signaling mechanism?

### 5. Scale testing
The fix was verified with unit tests. To fully validate, consider:
- Running a node with 100+ persistent peers that are all offline
- Monitoring `persistentConnReqs` map size over time
- Checking connmgr's internal state for orphaned ConnReqs
- Measuring memory and goroutine count under sustained offline-peer load

### 6. Other callers of cancelConnReqs
`cancelConnReqs` is called from several places:
- `ConnectToPeer` (Bug 1 fix)
- `OutboundPeerConnected` success path (existing, correct)
- `OutboundPeerConnected` inbound-exists path (Bug 4 fix)
- `InboundPeerConnected` (not analyzed — does it have similar issues?)
- `peerTerminationWatcher` (calls `connectToPersistentPeer`, not `cancelConnReqs` directly)

`InboundPeerConnected` was not analyzed in depth. It has a similar structure to `OutboundPeerConnected` and may have analogous issues.

### 7. `DisconnectPeer` interaction
When a user calls `lncli disconnect`, does it properly clean up ConnReqs? If the peer is persistent, will `peerTerminationWatcher` immediately recreate them? This is expected behavior for persistent peers, but it's worth verifying the cleanup/recreation cycle doesn't leak.

### 8. Startup behavior
At startup, `establishPersistentConnections` creates ConnReqs for all persistent peers. If the node crashes and restarts while there are accumulated ConnReqs, the restart creates fresh ones. But if the old connmgr process somehow survives (it shouldn't — this is defensive thinking), there could be connmgr-level duplicates. Worth verifying that connmgr is fully reset on restart.
