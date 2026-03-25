# Contract Resolvers Sweep Outputs

Contract resolvers are specialized state machines spawned by the [Channel
Arbitrator](202603251005-channel-arbitrator-manages-lifecycle.md) to handle the
complex logic required to sweep specific types of on-chain outputs. Because
lightning channel closures can involve multiple time-locked or conditional
outputs (like incoming HTLCs, outgoing HTLCs, anchors, or penalty sweeps), a
single sweeping strategy is insufficient.

Each contract resolver is tailored to a specific output type. For example, an
HTLC resolver knows whether it needs to wait for a time-lock to expire or if it
can immediately sweep using a known preimage. It tracks the progress of its
specific output, waits for necessary chain conditions, and provides the required
inputs (signatures, scripts, and preimages) to the
[Sweeper](202603251007-sweeper-batches-utxo-spends.md). Once the output is
successfully confirmed on-chain, the resolver marks itself resolved, allowing
the parent channel arbitrator to progress.

Tags: #architecture #dispute-resolution #on-chain

## References
- Coordinates with: [Sweeper](202603251007-sweeper-batches-utxo-spends.md)

## Backlinks
- [Contract Court Resolution](202603251003-Contract-Court-Resolution.md)
- [Channel Arbitrator Manages Lifecycle](202603251005-channel-arbitrator-manages-lifecycle.md)
- [Sweeper Batches Utxo Spends](202603251007-sweeper-batches-utxo-spends.md)
