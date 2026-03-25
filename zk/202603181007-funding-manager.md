# Funding Manager Orchestrates Channel Creation

The `funding` manager encapsulates the interactive state machine required to
negotiate channel parameters, exchange signatures, and bootstrap a channel via
an on-chain transaction. It isolates this multi-stage setup process,
coordinating with both the [Peer Network](202603181006-peer-network-management.md)
for message transmission and the [Wallet Abstraction](202603181003-lightning-wallet-abstraction.md)
for UTXO selection and signing (`FundingTxAssembler`).

This subsystem manages the complexity of different channel paradigms, such as
Anchor Channels, Zero-Conf Channels (where a channel is used before the funding
transaction is fully confirmed), and Taproot Channels (utilizing Schnorr
signatures and MuSig2 to improve privacy and lower fees). When a channel is
successfully funded and confirmed on-chain, the manager updates the [Channel
Database](202603181004-channel-state-database.md) to log its existence,
subsequently signaling the node to activate it for routing.

Tags: #architecture #channel-management #channel-establishment #state-machine

## References
- Builds states over: [Wallet Abstraction](202603181003-lightning-wallet-abstraction.md)
- Establishes for: [Lnd Architecture](202603181000-Lnd-Architecture.md)

## Backlinks
- [Lnd Architecture](202603181000-Lnd-Architecture.md)
- [Lightning Wallet Abstraction](202603181003-lightning-wallet-abstraction.md)
- [Autopilot Automates Liquidity](202603251011-autopilot-automates-liquidity.md)
