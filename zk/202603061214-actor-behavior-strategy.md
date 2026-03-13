# ActorBehavior: Logic as an Injected Strategy

An actor in LND contains no business logic of its own. The logic is a separate
object — an `ActorBehavior` — injected at construction time. The actor is purely
a runner: it owns the goroutine, the mailbox, and the lifecycle, but delegates
every message-processing decision to the behavior.

This separation has two practical consequences. First, the same actor
infrastructure can host entirely different logic by swapping the behavior —
there is nothing to subclass or override. Second, behaviors can be tested in
isolation by calling `Receive` directly, without spinning up a goroutine or a
mailbox at all.

A behavior is a single method: given the actor's context and a message, produce
a result. If the context is cancelled mid-processing, the behavior can detect it
and return early. `NewFunctionBehavior` wraps a plain function into a behavior,
avoiding even the need to define a named type for simple cases — the DLO itself
is implemented this way.

The behavior holds whatever state the actor needs. Because only one message is
processed at a time, that state is never accessed concurrently and requires no
locking. This is the mechanism by which the actor model's private-state
guarantee is realised in Go: not by language enforcement, but by structural
convention — the behavior's state is reachable only through the single goroutine
that calls `Receive`.

Tags: #architecture #actor #concurrency #lnd

## References
- Actor construction: [ActorSystem: Actor Lifecycle Management](202603061210-actor-system-lifecycle.md)
- Model property this realises:
  [The Actor Model: Origins and Core
  Properties](202603061200-actor-model-origins-properties.md)
- Collection: [Actor Pattern in LND](202603061215-Actor-Pattern-LND.md)

## Backlinks
- [Actor Pattern LND](zk/202603061215-Actor-Pattern-LND.md)
