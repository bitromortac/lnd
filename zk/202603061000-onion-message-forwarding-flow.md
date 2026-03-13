# Onion Message Forwarding Flow

When a node receives an `onion_message` destined for another peer, it executes a
forwarding pipeline that is deliberately stateless and silent on failure. The
full path through the system is captured in the diagram Zettel [Onion Message
Forwarding Flow Diagram](202603061005-onion-message-forwarding-diagram.md).

The raw wire message enters via the brontide peer's read loop. Because onion
messages are unreliable by design (see [Onion Message Delivers Application
Data](spec/202603040910-513-onion-message-delivers-application-data.md)), the
peer never blocks its read loop on processing. Instead it hands the message off
to a dedicated `OnionPeerActor` via a non-blocking `Tell` call. One actor is
spawned per peer at connection time, provided the remote peer advertises the
`option_onion_messages` feature during the `Init` handshake (see [Peer Brontide
Architecture](lnd/202602151330-peer-brontide-architecture.md)).

Inside the actor, `processOnionMessage` performs three sequential operations: it
decodes the raw Sphinx onion packet, processes it through the router to peel one
layer of encryption and derive the hop payload, then decrypts the
`encrypted_recipient_data` blob using the current `path_key`. The decrypted
blinded-route-data record tells the node either the next hop's explicit node ID
or a Short Channel ID (SCID) for the outbound link. When a SCID is provided, the
`GraphNodeResolver` maps it to a public key via an LRU cache backed by the
channel graph — a lookup that avoids repeated database I/O for frequently-seen
paths.

Once the routing decision is made the actor calls `PeerMessageSender.SendToPeer`
with the re-keyed onion (next `path_key` derived via `NextEphemeral`, or
overridden by `NextBlindingOverride` when two blinded paths are concatenated).
Regardless of whether the forward succeeds or fails, no error is returned to the
originating peer. The actor then dispatches an `OnionMessageUpdate` to the
subscription server so that any local observers (e.g. the RPC stream) can
inspect the hop — even for transit messages.

Tags: #architecture #lnd #onion-messages #privacy #networking

## References
- Protocol rules:
  [Onion Message Delivers Application
  Data](spec/202603040910-513-onion-message-delivers-application-data.md)
- Backpressure policy: [Onion Message Backpressure via Random Early Drop](202603061010-onion-message-backpressure-red.md)
- Blinding key derivation:
  [Route Blinding Conceals Recipient
  Identity](spec/202603040900-route-blinding-conceals-recipient-identity.md)
- Peer read loop host: [Peer Brontide Architecture](lnd/202602151330-peer-brontide-architecture.md)
- Flow diagram: [Onion Message Forwarding Flow Diagram](202603061005-onion-message-forwarding-diagram.md)

## Backlinks
- [Onion Message Forwarding Diagram](zk/202603061005-onion-message-forwarding-diagram.md)
- [Onion Message Backpressure Red](zk/202603061010-onion-message-backpressure-red.md)
- [Onion Message Receiver Flow](zk/202603061030-onion-message-receiver-flow.md)
- [Onion Messaging LND](zk/202603061040-Onion-Messaging-LND.md)
- [Actor Tell Fire And Forget](zk/202603061205-actor-tell-fire-and-forget.md)
- [Actor Standalone Usage](zk/202603061213-actor-standalone-usage.md)
