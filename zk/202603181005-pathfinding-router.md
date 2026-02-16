# Pathfinding Router Discovers Payment Routes

The `routing` component operates the `ControlTower` and evaluates the
authenticated `Graph` to discover the most cost-effective and reliable multihop
paths for outgoing payments. It utilizes the channel state and network
topology provided by the
[Channel Database](202603181004-channel-state-database.md) to calculate
optimal routes before handing them over to the
[HTLC Switch](202603181002-htlc-switch-routing.md).

It does not treat payments as single monolithic blobs. To increase success
rates, the router actively participates in cryptographic privacy overlays and
advanced routing techniques like Multi-Path Payments (MPP) or Atomic Multi-Path
Payments (AMP), where a payment is split into shards and routed over completely
disparate paths in the network graph. It also supports "Blinded Paths", which
obscure the final destination of a payment from intermediate forwarding nodes.
The router actively probes, learns from routing failures via its
`MissionControlQuerier`, and dynamically adjusts link probabilities.

Tags: #architecture #routing #pathfinding #graph-theory

## References
- Sends routes to: [HTLC Switch](202603181002-htlc-switch-routing.md)
- Uses graph from: [Channel Database](202603181004-channel-state-database.md)

## Backlinks
- [Lnd Architecture](202603181000-Lnd-Architecture.md)
- [Htlc Switch Routing](202603181002-htlc-switch-routing.md)
- [Channel State Database](202603181004-channel-state-database.md)
- [Pathfinding Router](202603181010-Pathfinding-Router.md)
