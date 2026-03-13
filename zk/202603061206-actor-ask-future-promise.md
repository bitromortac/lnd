# Ask: Request-Response via Future and Promise

Ask is the actor interaction pattern for when the caller needs a result. Before
depositing the message, the caller creates a promise and bundles it into the
mailbox envelope alongside the message. When the actor's behavior finishes
processing, it completes the promise with the result. The caller receives a
future immediately — it does not block while the actor works.

The future offers **three consumption modes**. The caller can wait
synchronously, blocking its goroutine until the result is ready or the context
is cancelled. It can register a transformation that produces a new future,
composing async operations into a pipeline without any goroutine blocking. Or it
can register a callback that fires once the result arrives.

**Example: pipelined enrichment.** A component asks a resolver actor for the
pubkey behind a channel ID. It receives a future immediately, then chains a
`ThenApply` that marks the result as verified if the key appears in a trusted
set. The enriched future is handed to the next stage. No goroutine blocks at
either step; the transformation runs automatically when the lookup completes.
The constraint to keep in mind: `ThenApply` maps `T → T`, so the type cannot
change across the transformation — it enriches or normalises, it does not
reshape.

This design bridges the synchronous and asynchronous worlds. Code that needs a
concrete value can wait for it; code building a processing pipeline can compose
futures without introducing blocking boundaries. If the actor has already
stopped when Ask is called, the future is completed immediately with a
termination error, so the caller is never left hanging.

The counterpart fire-and-forget pattern is [Tell: Fire-and-Forget Actor
Interaction](202603061205-actor-tell-fire-and-forget.md).

Tags: #architecture #actor #concurrency #lnd

## References
- Counterpart pattern: [Tell: Fire-and-Forget Actor Interaction](202603061205-actor-tell-fire-and-forget.md)
- Model context: [The Actor Model: Origins and Core Properties](202603061200-actor-model-origins-properties.md)
- System lifecycle (termination errors): [ActorSystem: Actor Lifecycle Management](202603061210-actor-system-lifecycle.md)
- Dead Letter Office: [Dead Letter Office: Observable Message Loss](202603061212-actor-dead-letter-office.md)

## Backlinks
- [Actor Model Origins Properties](zk/202603061200-actor-model-origins-properties.md)
- [Actor Tell Fire And Forget](zk/202603061205-actor-tell-fire-and-forget.md)
- [Actor Sealed Message Capability](zk/202603061207-actor-sealed-message-capability.md)
- [Actor Pattern LND](zk/202603061215-Actor-Pattern-LND.md)
