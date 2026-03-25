# Gossip Discovery Syncs Topology

The Lightning Network topology is decentralized; there is no central server
maintaining the map of active channels. The `discovery` subsystem, often called
the "gossiper," is responsible for the peer-to-peer synchronization of the
authenticated public channel graph.

When a new public channel is opened, or when the fee policies of an existing
channel change, the involved nodes broadcast cryptographically signed update
messages. The discovery subsystem listens to this network chatter, validates the
signatures against the blockchain to prevent spam, and updates its local
[Channel Database](202603181004-channel-state-database.md). By continuously
syncing this topology with connected peers, the gossiper provides the
[Pathfinding Router](202603181005-pathfinding-router.md) with an up-to-date,
global view of the network's liquidity and connectivity, which is essential for
routing payments successfully.

Tags: #architecture #gossip #lightning-network

## References
- Feeds data to: [Pathfinding Router](202603181005-pathfinding-router.md)
- Updates: [Channel Database](202603181004-channel-state-database.md)

## Backlinks
- [Lnd Architecture](202603181000-Lnd-Architecture.md)
