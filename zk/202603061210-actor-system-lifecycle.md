# ActorSystem: Actor Lifecycle Management

An `ActorSystem` is the runtime container for actors. Its core responsibility is
managing the full lifecycle of actors it owns: spawning them, tracking them, and
shutting them down gracefully.

When an actor is spawned, the system starts its event loop goroutine, registers
it in an internal registry of stoppable entities, and returns an `ActorRef`. The
caller holds only the reference, never the actor itself — the system owns the
concrete instance.

On shutdown, the system iterates all registered actors and cancels each one's
internal context. The actor's event loop detects the cancellation, exits, and
drains any messages still in the mailbox. Inflight messages are routed to the
Dead Letter Office (see [Dead Letter Office: Observable Message
Loss](202603061212-actor-dead-letter-office.md)); any pending `Ask` promises are
completed with a termination error so no caller is left blocked.

Tags: #architecture #actor #concurrency #lnd

## References
- Actor primitives: [The Actor Model: Origins and Core Properties](202603061200-actor-model-origins-properties.md)
- Service discovery: [Receptionist: Type-Safe Actor Service Discovery](202603061211-actor-receptionist-service-discovery.md)
- Dead Letter Office: [Dead Letter Office: Observable Message Loss](202603061212-actor-dead-letter-office.md)
- Standalone actors: [Actor Primitive Used Without an ActorSystem](202603061213-actor-standalone-usage.md)

## Backlinks
- [Actor Model Origins Properties](zk/202603061200-actor-model-origins-properties.md)
- [Actor Ask Future Promise](zk/202603061206-actor-ask-future-promise.md)
- [Actor Receptionist Service Discovery](zk/202603061211-actor-receptionist-service-discovery.md)
- [Actor Dead Letter Office](zk/202603061212-actor-dead-letter-office.md)
- [Actor Standalone Usage](zk/202603061213-actor-standalone-usage.md)
- [Actor Behavior Strategy](zk/202603061214-actor-behavior-strategy.md)
- [Actor Pattern LND](zk/202603061215-Actor-Pattern-LND.md)
