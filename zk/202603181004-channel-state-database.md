# Channel Database Persists Network State

The `channeldb` subsystem acts as the persistent storage layer for the
[Lnd Architecture](202603181000-Lnd-Architecture.md), securely logging channel
mutations and maintaining a local view of the global network graph. It separates
all state operations into dedicated buckets or sub-databases, like the
`ChannelStateDB`, to ensure atomicity.

By persisting these off-chain states, the database enables graceful recovery
during crash-faults and power cycles. If a node restarts, the
[HTLC Switch](202603181002-htlc-switch-routing.md) can rebuild its internal
circuit map from the database, and the daemon can seamlessly resume routing.
Furthermore, the database handles migration schemas to transition data models,
such as moving the legacy routing graph from a key-value store to a native SQL
backend, ensuring structural resilience as the codebase evolves.

Tags: #architecture #channel-state #database #storage

## References
- Used by: [HTLC Switch](202603181002-htlc-switch-routing.md)
- Backs up: [Pathfinding Router](202603181005-pathfinding-router.md)

## Backlinks
- [Lnd Architecture](202603181000-Lnd-Architecture.md)
- [Htlc Switch Routing](202603181002-htlc-switch-routing.md)
- [Pathfinding Router](202603181005-pathfinding-router.md)
- [Funding Manager](202603181007-funding-manager.md)
- [Payment Session Pathfinding](202603181013-payment-session-pathfinding.md)
- [Gossip Discovery Syncs Topology](202603251008-gossip-discovery-syncs-topology.md)
- [Invoice Registry Tracks Payments](202603251009-invoice-registry-tracks-payments.md)
