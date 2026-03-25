# Lightning Network Daemon Architecture

The Lightning Network Daemon (`lnd`) is a complete, modular implementation of
a Lightning Network node. Instead of a monolithic codebase, it coordinates
an ecosystem of decoupled subsystems that manage distinct lifecycle phases
of the network, ranging from peering and payment routing to on-chain state
enforcement.

## Architecture Map

```mermaid flowchart TD
    API[RPC Layer] --> Daemon[Daemon Process] Daemon --> Router[Pathfinding
    Router] Daemon --> Funding[Funding Manager] Router --> Switch[HTLC Switch]
    Switch --> Peer[Peer Network] Peer --> Switch Switch --> DB[(Channel
    Database)] Switch --> Court[Contract Court] Funding --> Wallet[Wallet
    Abstraction] Court --> Wallet
```

## Execution Core - **Orchestration:** [Daemon
Process](202603181001-lnd-daemon-process.md) wires
together modular interfaces without tight coupling.
- **Data Plane:** [HTLC Switch](202603181002-htlc-switch-routing.md) atomically
  forwards time-locked payments using [Sphinx Onion
  Unrolling](202603251013-sphinx-onion-unrolling.md).

- **Offline Security:** [Watchtower
  Architecture](202603251000-Watchtower-Architecture.md)
  monitors the chain for malicious channel breaches.
- **Authorization:** [Macaroon Auth](202603251010-macaroon-auth-secures-rpc.md)
  enforces granular permissions for the daemon's RPC endpoints.

## Network Layer - **Multiplexing:** [Peer
Network](202603181006-peer-network-management.md)
handles encrypted BOLT 8 transport via the [Brontide Noise
Protocol](202603251012-brontide-noise-protocol-handshake.md).
- **Topology:** [Pathfinding Router](202603181005-pathfinding-router.md)
  discovers routes over the authenticated channel graph.
- **Topology Sync:** [Gossip
  Discovery](202603251008-gossip-discovery-syncs-topology.md)
  maintains the global view of network liquidity.

## On-Chain Operations - **Channel Bootstrapping:**
[Funding Manager](202603181007-funding-manager.md) orchestrates the
interactive channel creation state machine.
- **Automated Provisioning:**
  [Autopilot](202603251011-autopilot-automates-liquidity.md) proactively
  establishes incoming network liquidity.
- **Payment Lifecycle:**
  [Invoice Registry](202603251009-invoice-registry-tracks-payments.md)
  tracks and validates incoming payments against user-issued requests.
- **Enforcement:** [Contract Court](202603251003-Contract-Court-Resolution.md)
  arbitrates disputes and sweeps funds on-chain.
- **State Persistence:**
  [Channel Database](202603181004-channel-state-database.md) securely logs
  channel mutations.
- **UTXO Management:**
  [Wallet Abstraction](202603181003-lightning-wallet-abstraction.md) abstracts
  underlying blockchain backends.

Tags: #architecture #lightning-network #software-architecture #diagram #entry-point

## References

## Backlinks - [Lnd Daemon Process](202603181001-lnd-daemon-process.md) -
[Htlc Switch Routing](202603181002-htlc-switch-routing.md) - [Lightning
Wallet Abstraction](202603181003-lightning-wallet-abstraction.md) -
[Channel State Database](202603181004-channel-state-database.md) - [Peer
Network Management](202603181006-peer-network-management.md) - [Funding
Manager](202603181007-funding-manager.md)
## Backlinks
- [Lnd Daemon Process](202603181001-lnd-daemon-process.md)
- [Htlc Switch Routing](202603181002-htlc-switch-routing.md)
- [Lightning Wallet Abstraction](202603181003-lightning-wallet-abstraction.md)
- [Channel State Database](202603181004-channel-state-database.md)
- [Peer Network Management](202603181006-peer-network-management.md)
- [Funding Manager](202603181007-funding-manager.md)
- [Contract Court Resolution](202603251003-Contract-Court-Resolution.md)
- [Invoice Registry Tracks Payments](202603251009-invoice-registry-tracks-payments.md)
- [Macaroon Auth Secures Rpc](202603251010-macaroon-auth-secures-rpc.md)
- [Autopilot Automates Liquidity](202603251011-autopilot-automates-liquidity.md)
- [Brontide Noise Protocol Handshake](202603251012-brontide-noise-protocol-handshake.md)
