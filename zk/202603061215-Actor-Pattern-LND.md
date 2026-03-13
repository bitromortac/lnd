# Actor Pattern in LND

LND implements the Actor Model as a typed, Go-native concurrency primitive. The
model gives each logical entity its own goroutine and mailbox, eliminating
shared mutable state and making concurrency concerns local to the actor
boundary.

## Concepts

- **Origins and properties** of the model:
  [The Actor Model: Origins and Core
  Properties](202603061200-actor-model-origins-properties.md)
- **Tell** — fire-and-forget, non-blocking dispatch:
  [Tell: Fire-and-Forget Actor
  Interaction](202603061205-actor-tell-fire-and-forget.md)
- **Ask** — request-response via Future and Promise:
  [Ask: Request-Response via Future and
  Promise](202603061206-actor-ask-future-promise.md)
- **Capability restriction** — sealed message types and reference narrowing:
  [Sealed Message Interface and Capability
  Restriction](202603061207-actor-sealed-message-capability.md)
- **ActorSystem** — lifecycle management, spawn and shutdown:
  [ActorSystem: Actor Lifecycle
  Management](202603061210-actor-system-lifecycle.md)
- **Receptionist** — type-safe service discovery by key:
  [Receptionist: Type-Safe Actor Service
  Discovery](202603061211-actor-receptionist-service-discovery.md)
- **Dead Letter Office** — observable record of undeliverable messages:
  [Dead Letter Office: Observable Message
  Loss](202603061212-actor-dead-letter-office.md)
- **Standalone actors** — using the primitive without an ActorSystem:
  [Actor Primitive Used Without an
  ActorSystem](202603061213-actor-standalone-usage.md)
- **ActorBehavior** — logic as an injected strategy, testable in isolation:
  [ActorBehavior: Logic as an Injected
  Strategy](202603061214-actor-behavior-strategy.md)

## Applied Use: Onion Messaging

The onion message subsystem is the primary consumer of this pattern. Each peer
that advertises `option_onion_messages` gets its own actor instance, giving
per-peer isolation and per-peer backpressure. The actor collection is documented
at [Onion Messaging in LND](202603061040-Onion-Messaging-LND.md).

Tags: #entry-point #architecture #actor #lnd #concurrency

## References

## Backlinks
- [Onion Messaging LND](zk/202603061040-Onion-Messaging-LND.md)
- [Actor Behavior Strategy](zk/202603061214-actor-behavior-strategy.md)
