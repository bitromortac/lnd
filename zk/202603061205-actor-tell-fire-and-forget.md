# Tell: Fire-and-Forget Actor Interaction

Tell is the one-way actor interaction pattern: the caller deposits a message and
expects no response. It never waits for the actor to process the message —
"fire-and-forget" describes the response contract, not the blocking behaviour.

Whether Tell returns immediately depends on the mailbox. With a bounded channel
mailbox, Tell blocks until a slot is available, the caller's context is
cancelled, or the actor stops. A full mailbox with a long-lived context will
hold the caller. With a backpressure mailbox, Tell never blocks — excess
messages are dropped before they enter the queue. The onion peer actors use the
latter precisely because blocking the peer's read handler would stall all
message processing for that connection.

This is the natural fit for events and notifications: informing the actor that
something happened without needing a confirmation. In LND, the peer's read
handler uses Tell to hand an arriving onion message to the per-peer actor, then
returns to reading the next wire message without waiting for routing to
complete.

A `TellOnlyRef` is a reference that exposes *only* this capability. It is used
when a caller has no business performing request-response with an actor —
handing it a narrow reference makes that constraint structural and
compiler-enforced rather than a convention. The relationship between capability
restriction and message-type safety is covered in [Sealed Message Interface and
Capability Restriction](202603061207-actor-sealed-message-capability.md).

Tags: #architecture #actor #concurrency #lnd

## References
- Counterpart pattern: [Ask: Request-Response via Future and Promise](202603061206-actor-ask-future-promise.md)
- Model context: [The Actor Model: Origins and Core Properties](202603061200-actor-model-origins-properties.md)
- Onion actor (Tell consumer): [Onion Message Forwarding Flow](202603061000-onion-message-forwarding-flow.md)
- Backpressure mailbox (non-blocking Tell): [Onion Message Backpressure via Random Early Drop](202603061010-onion-message-backpressure-red.md)

## Backlinks
- [Actor Model Origins Properties](zk/202603061200-actor-model-origins-properties.md)
- [Actor Ask Future Promise](zk/202603061206-actor-ask-future-promise.md)
- [Actor Sealed Message Capability](zk/202603061207-actor-sealed-message-capability.md)
- [Actor Dead Letter Office](zk/202603061212-actor-dead-letter-office.md)
- [Actor Pattern LND](zk/202603061215-Actor-Pattern-LND.md)
