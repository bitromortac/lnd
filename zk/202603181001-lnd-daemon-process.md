# Daemon Process Orchestrates Subsystems

The top-level `lnd` daemon acts as an orchestrator, bootstrapping configuration
and wiring together a vast ecosystem of modular components. Rather than having
subsystems like the wallet and the HTLC switch tightly coupled, the daemon
injects specialized interfaces (e.g., `WalletController`, `HTLCSwitch`,
`ControlTower`) into each service.

This orchestration decouples the execution layer from the protocol logic. It
enables [Lightning Wallet
Abstraction](202603181003-lightning-wallet-abstraction.md) to function
independently from the data plane, allowing components to be tested in isolation
or swapped entirely without disrupting the broader [Lnd
Architecture](202603181000-Lnd-Architecture.md).

Tags: #architecture #lightning-network #daemon #orchestration

## References
- Coordinates the: [HTLC Switch](202603181002-htlc-switch-routing.md)
- Coordinates the: [Wallet Abstraction](202603181003-lightning-wallet-abstraction.md)

## Backlinks
- [Lnd Architecture](202603181000-Lnd-Architecture.md)
- [Lightning Wallet Abstraction](202603181003-lightning-wallet-abstraction.md)
