# Backpressure Queue Context Diagram

This diagram illustrates the structural relationships between the core queue
components, the mailbox abstraction, and the actor system used for onion
messaging.

The [backpressure mechanism](202603061010-onion-message-backpressure-red.md)
integrates with the actor model by wrapping a generic channel-based queue inside
a mailbox interface. This allows actors to transparently shed load using drop
predicates like Random Early Detection.

```mermaid
classDiagram
    class DropCheckFunc
    DropCheckFunc : func(queueLen int) bool

    class DropPredicate
    DropPredicate : func(queueLen int, item T) bool

    class BackpressureQueue
    BackpressureQueue : -ch chan T
    BackpressureQueue : -dropPredicate DropPredicate
    BackpressureQueue : +Enqueue(ctx, item) error
    BackpressureQueue : +TryEnqueue(item) bool
    BackpressureQueue : +Dequeue(ctx) Result

    class Mailbox
    <<interface>> Mailbox
    Mailbox : +Send(ctx, env) bool
    Mailbox : +TrySend(env) bool
    Mailbox : +Receive(ctx)
    Mailbox : +Close()

    class BackpressureMailbox
    BackpressureMailbox : -queue BackpressureQueue
    BackpressureMailbox : +Send(ctx, env) bool
    BackpressureMailbox : +TrySend(env) bool
    BackpressureMailbox : +Receive(ctx) iter

    class Actor
    Actor : -mailbox Mailbox

    class OnionPeerActor
    OnionPeerActor : -peerPubKey bytes
    OnionPeerActor : +Receive(ctx, req) Result

    DropPredicate ..> BackpressureQueue : determines drops
    DropCheckFunc ..> DropPredicate : AsDropPredicate
    BackpressureQueue *-- BackpressureMailbox : wrapped by
    Mailbox <|-- BackpressureMailbox : implements
    Mailbox --* Actor : used by
    OnionPeerActor ..|> Actor : behavior implementation
```

Tags: #diagram #architecture #lnd #concurrency #onion-messages #skip-lint

## References

## Backlinks
- [Onion Message Backpressure Red](zk/202603061010-onion-message-backpressure-red.md)
