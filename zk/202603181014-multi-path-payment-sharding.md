# Payment Sharding Bypasses Liquidity Bottlenecks

The Lightning Network routing graph is constrained by the liquidity limits of
individual channels. When a single logical payment exceeds the capacity of any
single path to the destination, the
[Pathfinding Router](202603181010-Pathfinding-Router.md) employs multi-path
sharding strategies (MPP or AMP) to bypass these bottlenecks.

The `paymentLifecycle` utilizes a `ShardTracker` to split the total payment
amount into smaller, independent HTLC shards that can traverse completely
disparate routes across the network. These shards are propagated by the
[HTLC Switch](202603181002-htlc-switch-routing.md) in parallel, and the
receiving node aggregates them, waiting for the full threshold amount to arrive
before settling the payment atomically using the shared preimage or secret
derivation. This not only increases the probability of large payments
succeeding, but it also improves the overall capital efficiency of the network.

Tags: #routing #pathfinding #payment

## References
- Invoked by: [Payment Lifecycle](202603181011-payment-lifecycle-state-machine.md)
- Routed by: [Pathfinding Router](202603181010-Pathfinding-Router.md)

## Backlinks
- [Pathfinding Router](202603181010-Pathfinding-Router.md)
