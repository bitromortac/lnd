# Channel Arbitrator Manages Lifecycle

The channel arbitrator is a state machine that governs the on-chain resolution
of a single disputed channel. It is spawned by the [Chain
Arbitrator](202603251004-chain-arbitrator-dispatches-events.md) when a
unilateral close or breach is detected on the blockchain.

Once activated, the channel arbitrator evaluates the state of the closed channel
and determines which outputs need to be claimed. It does not perform the
sweeping itself; instead, it delegates the specific claiming logic by spawning
multiple [Contract Resolvers](202603251006-contract-resolvers-sweep-outputs.md)
tailored to the types of outputs present (e.g., HTLCs, anchor outputs, or
commitment sweeps). The channel arbitrator coordinates these resolvers,
progressing its own state machine until the channel is fully resolved and all
funds are recovered.

Tags: #architecture #dispute-resolution #on-chain

## References

## Backlinks
- [Contract Court Resolution](202603251003-Contract-Court-Resolution.md)
- [Chain Arbitrator Dispatches Events](202603251004-chain-arbitrator-dispatches-events.md)
- [Contract Resolvers Sweep Outputs](202603251006-contract-resolvers-sweep-outputs.md)
