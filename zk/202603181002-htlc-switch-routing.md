# HTLC Switch Orchestrates Payment Forwarding

The `htlcswitch` acts as the Lightning Network's data plane, much like a network
router for physical packets. It is the central hub that receives time-locked
payments (HTLCs) from peers and atomically forwards them across the network
using onion routing.

It maintains an active `ChannelLink` for each peer connection and logs the state
of in-flight payments via the `ForwardingLog` and `CircuitMap`. The switch uses
these circuits to correctly unroll the onion routing packets and assign incoming
payments to outgoing channels. By abstracting the complex off-chain state
machine, the [Lnd Architecture](202603181000-Lnd-Architecture.md) can rely on
the switch to correctly propagate updates and enforce atomicity between
incoming and outgoing links without blocking.

Tags: #architecture #routing #htlc #payment

## References
- Depends on: [Channel Database](202603181004-channel-state-database.md)
- Routed by: [Pathfinding Router](202603181005-pathfinding-router.md)

## Backlinks
- [Lnd Architecture](202603181000-Lnd-Architecture.md)
- [Lnd Daemon Process](202603181001-lnd-daemon-process.md)
- [Channel State Database](202603181004-channel-state-database.md)
- [Pathfinding Router](202603181005-pathfinding-router.md)
- [Peer Network Management](202603181006-peer-network-management.md)
- [Contract Court Arbitration](202603181008-contract-court-arbitration.md)
- [Payment Lifecycle State Machine](202603181011-payment-lifecycle-state-machine.md)
- [Multi Path Payment Sharding](202603181014-multi-path-payment-sharding.md)
- [Blinded Paths Privacy](202603181015-blinded-paths-privacy.md)
