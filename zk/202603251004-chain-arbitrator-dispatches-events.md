# Chain Arbitrator Dispatches Events

The chain arbitrator serves as the global dispatch mechanism within the
[Contract Court Resolution](202603251003-Contract-Court-Resolution.md)
subsystem. Instead of having every disputed channel individually poll the
blockchain, this centralized component ingests new blocks and reorganizations.

When a block arrives, the chain arbitrator scans it for relevant state changes
and maps those events to active channel disputes. By centralizing the chain
observation, the daemon conserves resources and ensures that all active [channel
arbitrators](202603251005-channel-arbitrator-manages-lifecycle.md) receive
synchronized, verified block signals to advance their internal state machines.

Tags: #architecture #dispute-resolution #on-chain

## References

## Backlinks
- [Contract Court Resolution](202603251003-Contract-Court-Resolution.md)
- [Channel Arbitrator Manages Lifecycle](202603251005-channel-arbitrator-manages-lifecycle.md)
