# Wallet Layer Abstracts On-Chain Operations

The `lnwallet` subsystem completely abstracts the choice of underlying
blockchain backend for the [Lnd Architecture](202603181000-Lnd-Architecture.md).
By exposing unified interfaces such as `WalletController` and `BlockChainIO`, it
handles all interactions related to UTXOs, fee estimation, and transaction
signing without tightly coupling the node to a specific implementation like
`bitcoind` or `neutrino`.

This modular approach also enables support for advanced features like remote
signing, where a watch-only wallet can securely delegate transaction signing
(`MusigSession` or ECDSA) to an isolated, secure signer component. Ultimately,
the wallet abstraction ensures that channel states can be cleanly and reliably
enforced on the underlying chain when required by components like the [Contract
Court](202603181008-contract-court-arbitration.md) or the [Funding
Manager](202603181007-funding-manager.md).

Tags: #architecture #wallet #on-chain #utxo-management

## References
- Invoked by: [Daemon Process](202603181001-lnd-daemon-process.md)
- Funds via: [Funding Manager](202603181007-funding-manager.md)

## Backlinks
- [Lnd Architecture](202603181000-Lnd-Architecture.md)
- [Lnd Daemon Process](202603181001-lnd-daemon-process.md)
- [Funding Manager](202603181007-funding-manager.md)
- [Contract Court Arbitration](202603181008-contract-court-arbitration.md)
