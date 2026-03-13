# Onion Message Receiver Flow

When the local node is the final destination of a blinded onion path, delivery
happens through the same per-peer actor pipeline as forwarding — no separate
channel is needed. What differs is the routing outcome: once the Sphinx
processor signals that there is no next hop, the actor takes the delivery branch
and surfaces the decrypted payload to the subscription layer instead of relaying
it outward. The full sequence appears in [Onion Message Receiver Flow
Diagram](202603061035-onion-message-receiver-diagram.md).

Delivery carries a privacy constraint inherited from route blinding: the
subscriber learns only the identity of the *immediate* sending peer, not the
originator. The sender's identity is hidden by design. The payload may include a
reply path if the sender wishes to allow an anonymous response, and
application-level data in TLV fields. The subscriber receives the raw encrypted
recipient data opaquely, so it can perform its own verification of the path
secret if it needs to — see [Path ID Validation is Application's
Resp](202603061220-onion-message-path-id-application-responsibility.md).

Because onion messages are unreliable by design (see [Onion Message Delivers
Application
Data](spec/202603040910-513-onion-message-delivers-application-data.md)),
delivery carries no storage or acknowledgement. A subscriber that is not
actively listening when a message arrives will miss it. Updates flow through a
fan-out pub-sub server to any number of registered listeners, including the RPC
layer that forwards them to external gRPC clients.

Tags: #architecture #lnd #onion-messages #privacy

## References
- Forwarding counterpart: [Onion Message Forwarding Flow](202603061000-onion-message-forwarding-flow.md)
- Receiver flow diagram: [Onion Message Receiver Flow Diagram](202603061035-onion-message-receiver-diagram.md)
- Path ID responsibility: [Path ID Validation Is the Application's Responsibility](202603061220-onion-message-path-id-application-responsibility.md)
- Protocol final-hop rules: [Onion Message Requirements](spec/202603041010-onion-message-requirements.md)

## Backlinks
- [Onion Message Receiver Diagram](zk/202603061035-onion-message-receiver-diagram.md)
- [Onion Messaging LND](zk/202603061040-Onion-Messaging-LND.md)
- [Onion Message Path Id Application Responsibility](zk/202603061220-onion-message-path-id-application-responsibility.md)
