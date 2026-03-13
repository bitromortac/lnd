# Actor Primitive Used Without an ActorSystem

The `ActorSystem` is a convenience layer — lifecycle registry, receptionist, and
dead letter routing in one place. The underlying `Actor[M,R]` primitive can be
used standalone when those services are not needed or when the actor's lifecycle
is already managed by an external component.

The onion peer actors are the primary example. One is created per peer
connection that advertises onion message support, held directly in the peer
struct, and stopped when the peer disconnects. The actor count is dynamic and
its lifetime is tied to a connection — not to a central system. Routing the
actors through an `ActorSystem` would add overhead and coupling without benefit.

This pattern is appropriate whenever the actor count is variable, the lifecycle
is externally determined, and service discovery is not required. The full system
is reserved for actors that need to be centrally managed, discoverable by key,
or whose termination must be coordinated with a larger shutdown sequence.

Tags: #architecture #actor #concurrency #lnd

## References
- Full system alternative: [ActorSystem: Actor Lifecycle Management](202603061210-actor-system-lifecycle.md)
- Onion actor example: [Onion Message Forwarding Flow](202603061000-onion-message-forwarding-flow.md)
- Backpressure in standalone actors: [Onion Message Backpressure via Random Early Drop](202603061010-onion-message-backpressure-red.md)

## Backlinks
- [Actor System Lifecycle](zk/202603061210-actor-system-lifecycle.md)
- [Actor Pattern LND](zk/202603061215-Actor-Pattern-LND.md)
