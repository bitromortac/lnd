# Sweeper Batches UTXO Spends

The sweeper is a specialized component within the [Contract Court Resolution](202603251003-Contract-Court-Resolution.md)
that is responsible for maximizing fee efficiency and ensuring reliable
confirmations for on-chain spends. It aggregates time-sensitive spends provided
by the [Contract Resolvers](202603251006-contract-resolvers-sweep-outputs.md)
and constructs the final sweep transactions.

Rather than publishing an individual transaction for every single time-locked or
conditional output that needs claiming, the sweeper batches them together when
possible. By combining multiple UTXOs into a single transaction, the sweeper
significantly reduces the overall on-chain fee overhead. It also dynamically
adjusts fee rates using Replace-By-Fee (RBF) or Child-Pays-For-Parent (CPFP)
when a transaction remains unconfirmed and time-locks are nearing expiration,
ensuring the node does not lose funds due to network congestion.

Tags: #architecture #on-chain

## References
- Used by: [Contract Resolvers](202603251006-contract-resolvers-sweep-outputs.md)

## Backlinks
- [Contract Court Resolution](202603251003-Contract-Court-Resolution.md)
- [Contract Resolvers Sweep Outputs](202603251006-contract-resolvers-sweep-outputs.md)
