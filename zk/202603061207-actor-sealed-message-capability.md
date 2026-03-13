# Sealed Message Interface and Capability Restriction

Two complementary mechanisms ensure that only the right types of interaction
reach an actor.

The first is the sealed message interface. An unexported marker method on the
`Message` type means only types that explicitly opt in — by embedding the
provided base type — can be sent to an actor at all. Arbitrary values cannot
accidentally flow into the message pipeline; the constraint is enforced at
compile time rather than by convention.

The second is reference narrowing. A full actor reference exposes both
fire-and-forget and request-response. A `TellOnlyRef` exposes only
fire-and-forget. When a component has no business performing request-response
with an actor, it receives the narrower reference. It becomes structurally
impossible — not just inadvisable — for that component to block on a result it
should not be waiting for.

Together these form a layered application of the principle of least privilege:
first, restrict what can be a message; second, restrict what the caller can do
with it.

Tags: #architecture #actor #concurrency #lnd

## References
- Tell pattern (uses TellOnlyRef): [Tell: Fire-and-Forget Actor Interaction](202603061205-actor-tell-fire-and-forget.md)
- Ask pattern (full capability): [Ask: Request-Response via Future and Promise](202603061206-actor-ask-future-promise.md)
- Model context: [The Actor Model: Origins and Core Properties](202603061200-actor-model-origins-properties.md)

## Backlinks
- [Actor Tell Fire And Forget](zk/202603061205-actor-tell-fire-and-forget.md)
- [Actor Pattern LND](zk/202603061215-Actor-Pattern-LND.md)
