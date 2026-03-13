# Onion Message Receiver Flow Diagram

Sequence for the case where the local node is the final hop. See
[Onion Message Receiver Flow](202603061030-onion-message-receiver-flow.md)
for prose explanation.

```mermaid
sequenceDiagram
    participant Prev as "Prev Peer (TCP)"
    participant Peer as "brontide readHandler"
    participant PeerActor as OnionPeerActor
    participant Router as SphinxRouter
    participant Sub as "onionMessageServer (subscribe)"
    participant RPC as "rpcServer stream"
    participant App as "Subscriber (application)"

    Prev->>Peer: OnionMessage(path_key, onion_blob)
    Peer->>PeerActor: Tell(ctx, Request(msg))

    PeerActor->>Router: ProcessOnionPacket(pkt, path_key)
    Router-->>PeerActor: ProcessedPacket(action=ExitNode, payload)

    PeerActor->>Router: DecryptBlindedHopData(path_key, encrypted_data)
    Router-->>PeerActor: BlindedRouteData(path_id?, ...)

    Note over PeerActor: deliverAction branch (ExitNode)
    Note over PeerActor: no SendToPeer call

    PeerActor->>Sub: SendUpdate(peer, path_key, onion_blob, reply_path?,
    custom_records)

    Sub->>RPC: Updates() channel receives update
    RPC->>App: stream.Send(lnrpc.OnionMessageUpdate)

    Note over App: path_id validation is application responsibility
```

Tags: #diagram #architecture #lnd #onion-messages #privacy

## References
- Prose: [Onion Message Receiver Flow](202603061030-onion-message-receiver-flow.md)

## Backlinks
- [Onion Message Receiver Flow](zk/202603061030-onion-message-receiver-flow.md)
