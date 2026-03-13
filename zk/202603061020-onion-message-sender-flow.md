# Onion Message Sender Flow

Sending an onion message begins entirely outside LND's routing engine — the
caller is responsible for constructing the Sphinx onion before handing it to the
node. LND's role is purely transport: it wraps the pre-built blob in an
`OnionMessage` wire message and delivers it to a connected peer.

## Construction (caller's responsibility)

The sender must build the blinded path and onion packet before calling the RPC.
The steps, as exercised by itests, are:

1. Obtain the recipient's (or introduction node's) public key and build
   `BlindedRouteData` for each hop — specifying either an explicit
   `next_node_id` or a SCID for non-final hops, and empty data for the final
   hop.
2. Call `sphinx.BuildBlindedPath(sessionKey, hops)` to derive blinded node IDs,
   encrypt routing blobs per hop, and produce a `BlindedPathInfo` containing the
   `BlindingPoint` (the initial `path_key`).
3. Convert the blinded path to a sphinx hop-by-hop path via
   `route.OnionMessageBlindedPathToSphinxPath`, appending any final-hop TLV
   application payloads (e.g. `invoice_request` bytes) to the last hop.
4. Call `sphinx.NewOnionPacket` with a fresh per-message session key and no
   associated data to produce the layered ciphertext blob.

For concatenated paths (two independently-constructed blinded segments joined at
an introduction node), the sender sets `NextBlindingOverride` in the
introduction node's route data to the receiver's `BlindingPoint`, then appends
the receiver's blinded hops. The forwarding node at the junction switches to the
new path key at that hop.

## Transmission (LND's responsibility)

The RPC `SendOnionMessage` receives `{peer, path_key, onion_blob}` and delegates
straight to `server.SendOnionMessage`. The server looks up the peer by public
key, waits for its active signal (ensuring the connection is ready), constructs
`lnwire.OnionMessage{PathKey, OnionBlob}`, and calls
`peer.SendMessageLazy(lowPriority=true, msg)`. No acknowledgement is returned —
the send is fire-and-forget. The flow is shown in [Onion Message Sender Flow
Diagram](202603061025-onion-message-sender-diagram.md).

Tags: #architecture #lnd #onion-messages #privacy

## References
- Blinding path construction: [Route Blinding Conceals Recipient Identity](spec/202603040900-route-blinding-conceals-recipient-identity.md)
- Reply path mechanism: [Reply Path Enables Anonymous Response](spec/202603040920-reply-path-enables-anonymous-response.md)
- BOLT 12 consumer of this flow: [Bolt12 Offer Flow Uses Onion Messaging](spec/202603041020-bolt12-offer-flow-uses-onion-messaging.md)
- Sender flow diagram: [Onion Message Sender Flow Diagram](202603061025-onion-message-sender-diagram.md)
- Peer transport layer: [Peer Brontide Architecture](lnd/202602151330-peer-brontide-architecture.md)

## Backlinks
- [Onion Message Sender Diagram](zk/202603061025-onion-message-sender-diagram.md)
- [Onion Messaging LND](zk/202603061040-Onion-Messaging-LND.md)
