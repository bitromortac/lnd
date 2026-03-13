# Receptionist: Type-Safe Actor Service Discovery

The Receptionist is a typed registry that decouples callers from the actors they
need. Instead of holding a direct reference to a specific actor, a component
holds a `ServiceKey` — a named, type-safe handle for a category of actors. Any
component that holds the key can look up all live references registered under
it, without any compile-time dependency on the actors themselves.

`FindInReceptionist` returns all references for a key at once, enabling fan-out
to multiple actors of the same kind and dynamic discovery of actors whose count
or identity is not known at construction time.

This is the actor-model analogue of service registries found in distributed
systems — Erlang's process groups, Akka's Cluster Receptionist, Kubernetes
services. In LND the pattern is applied intra-process: it decouples subsystems
that need to communicate without being wired together directly.

Tags: #architecture #actor #concurrency #lnd

## References
- System that hosts it: [ActorSystem: Actor Lifecycle Management](202603061210-actor-system-lifecycle.md)
- Actor primitives: [The Actor Model: Origins and Core Properties](202603061200-actor-model-origins-properties.md)

## Backlinks
- [Actor System Lifecycle](zk/202603061210-actor-system-lifecycle.md)
- [Actor Pattern LND](zk/202603061215-Actor-Pattern-LND.md)
