# Onion Message Backpressure via Random Early Drop

Onion message forwarding is cheap but unbounded in arrival rate. A malicious or
misconfigured peer could flood a node with messages, starving legitimate
traffic. LND defends against this with a two-layer backpressure strategy: a
per-peer actor mailbox with a fixed capacity, and a Random Early Drop (RED)
predicate that begins shedding load before the mailbox is full.

## The Mailbox Layer

Each peer connection has its own `OnionPeerActor` backed by a
`BackpressureMailbox`. The mailbox wraps a `BackpressureQueue` — a
fixed-capacity buffered channel. The default capacity is 50 messages
(`DefaultOnionMailboxSize`). Because each peer has a private mailbox, a flood
from one peer cannot exhaust capacity allocated to others. If the actor's
context is cancelled (peer disconnect), any blocked send unblocks immediately.

## The RED Layer

The drop predicate is wired up at server startup using `queue.RandomEarlyDrop`
with thresholds derived from the mailbox size. The current defaults are:

- **min threshold: 40** (`DefaultMinREDThreshold`) — below this depth, no
  drops occur.
- **max threshold: 50** (`DefaultOnionMailboxSize`) — at or above this depth,
  every message is dropped.

Between the two thresholds the drop probability scales linearly:

p = (depth − 40) / (50 − 40)

At depth 45 there is a 50 % chance of dropping any arriving message. This ramp
prevents a sudden cliff at capacity, smooths queue occupancy under sustained
load, and mirrors the RED algorithm used in TCP congestion control.

A dropped message is silently discarded — consistent with the protocol's
unreliability guarantee in [Onion Message Delivers Application
Data](spec/202603040910-513-onion-message-delivers-application-data.md).

Tags: #architecture #lnd #onion-messages #concurrency #networking

## References
- System Context: [Backpressure Queue Context Diagram](202603130900-backpressure-queue-context-diagram.md)
- Process Flow: [Backpressure Queue Execution Flow](202603130905-backpressure-queue-execution-flow-diagram.md)
- Mailbox host: [Onion Message Forwarding Flow](202603061000-onion-message-forwarding-flow.md)
- General mailbox pattern: [Mailbox Architecture](lnd/202602151340-mailbox-architecture.md)
- Protocol silent-drop rule: [Onion Message Delivers Application Data](spec/202603040910-513-onion-message-delivers-application-data.md)

## Backlinks
- [Onion Message Forwarding Flow](zk/202603061000-onion-message-forwarding-flow.md)
- [Onion Messaging LND](zk/202603061040-Onion-Messaging-LND.md)
- [Actor Tell Fire And Forget](zk/202603061205-actor-tell-fire-and-forget.md)
- [Actor Standalone Usage](zk/202603061213-actor-standalone-usage.md)
- [Backpressure Queue Context Diagram](zk/202603130900-backpressure-queue-context-diagram.md)
- [Backpressure Queue Execution Flow Diagram](zk/202603130905-backpressure-queue-execution-flow-diagram.md)
