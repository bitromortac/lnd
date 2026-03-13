# Onion Messaging in LND

This collection maps LND's implementation of the BOLT #4 onion messaging
sub-protocol. The protocol foundation and privacy requirements are specified in
[Bolt 4 Onion Messaging](spec/202603041000-Bolt-4-Onion-Messaging.md).

## Flows

- **Forwarding:** An arriving message is peeled by the Sphinx router, re-keyed,
  and relayed to the next peer. The per-peer actor model and backpressure policy
  are described in
  [Onion Message Forwarding
  Flow](202603061000-onion-message-forwarding-flow.md).
- **Backpressure:** Load shedding via Random Early Drop protects the node from
  message floods without violating the protocol's silent-drop contract. See
  [Onion Message Backpressure via Random
  Early Drop](202603061010-onion-message-backpressure-red.md).
- **Sending:** The caller constructs the full Sphinx onion and blinded path;
  LND's role is limited to transport to a directly-connected peer. Covered in
  [Onion Message Sender Flow](202603061020-onion-message-sender-flow.md).
- **Receiving:** The final-hop payload is surfaced to subscribers via a pub-sub
  server; messages are not persisted. Path ID validation is left to the
  application. See
  [Onion Message Receiver Flow](202603061030-onion-message-receiver-flow.md).

## Key Design Properties

Messages are unreliable by design: no acknowledgements, no retries, no
persistence. The per-peer actor-with-mailbox model isolates flooding from one
peer, and RED probabilistically sheds load before capacity is reached. Path
finding for multi-hop sends is not yet implemented in LND.

Tags: #entry-point #architecture #lnd #onion-messages #privacy

## References
- Protocol specification: [Bolt 4 Onion Messaging](spec/202603041000-Bolt-4-Onion-Messaging.md)
- LND system context: [Lnd Architecture](lnd/202602151305-LND-Architecture.md)
- Actor primitive used here: [Actor Pattern in LND](202603061215-Actor-Pattern-LND.md)

## Backlinks
- [LND Architecture](lnd/202602151305-LND-Architecture.md)
- [Actor Pattern LND](zk/202603061215-Actor-Pattern-LND.md)
