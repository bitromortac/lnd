# Contract Court Arbitrates Disputed Channels

The `contractcourt` is the on-chain enforcement mechanism of the
[Lnd Architecture](202603181000-Lnd-Architecture.md). Channel closures are not
simply socket disconnects. When a peer misbehaves, goes offline, or broadcasts
an outdated state, the contract court watches the blockchain for breaches and
resolves unilateral closes.

It acts as an arbiter that sweeps contested funds, such as in-flight
[HTLCs](202603181002-htlc-switch-routing.md), back to the user's wallet via
the `UtxoSweeper` and `ContractResolver` interfaces. For example, if an HTLC
is in an accepted but unsettled state (a "hold invoice") and the channel is
closed, the daemon must manage that exact lock time against the blockchain to
prevent loss. It also groups on-chain spends to save fees (HTLC aggregation)
and monitors impending lease or HTLC expiries to forcibly publish transactions
if a peer is unresponsive.

Tags: #architecture #dispute-resolution #on-chain #security

## References
- Arbitrates: [HTLC Switch](202603181002-htlc-switch-routing.md)
- Funds swept to: [Wallet Abstraction](202603181003-lightning-wallet-abstraction.md)

## Backlinks
- [Lnd Architecture](202603181000-Lnd-Architecture.md)
- [Lightning Wallet Abstraction](202603181003-lightning-wallet-abstraction.md)
