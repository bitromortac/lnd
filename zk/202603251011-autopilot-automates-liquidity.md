# Autopilot Automates Channel Liquidity

Autopilot is a heuristic-driven automated channel provisioning system within the
`lnd` architecture. Instead of requiring users to manually select and open
connections to other nodes, Autopilot analyzes the network graph and attempts to
allocate a portion of the node's local on-chain funds into Lightning channels.

The system uses heuristics to determine optimal peers—such as finding nodes with
high centrality or stable uptimes—to maximize routing efficiency and
connectivity to the broader network. Once it identifies favorable candidates,
Autopilot automatically requests the [Funding
Manager](202603181007-funding-manager.md) to open channels with those peers.
This automation greatly lowers the barrier to entry for running a routing node,
enabling users to seamlessly transition from passive wallet operators to active
network participants without deep topological knowledge.

Tags: #architecture #automation #lightning-network

## References
- Interacts with: [Funding Manager](202603181007-funding-manager.md)

## Backlinks
- [Lnd Architecture](202603181000-Lnd-Architecture.md)
