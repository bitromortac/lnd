# Onion Message Forwarding Flow Diagram

Sequence of system boundaries crossed when a transit onion message arrives and
is forwarded to the next peer. See [Onion Message Forwarding
Flow](202603061000-onion-message-forwarding-flow.md) for prose explanation.

```mermaid
sequenceDiagram
    participant Prev as "Prev Peer (TCP)"
    participant Peer as "brontide readHandler"
    participant PeerActor as OnionPeerActor
    participant Router as SphinxRouter
    participant Resolver as GraphNodeResolver
    participant Graph as "ChannelGraph (DB)"
    participant Sender as "server.SendToPeer"
    participant Next as "Next Peer (TCP)"
    participant Sub as "onionMessageServer (subscribe)"

    Prev->>Peer: OnionMessage(path_key, onion_blob)
    Peer->>PeerActor: Tell(ctx, Request(msg)) non-blocking

    PeerActor->>Router: ProcessOnionPacket(pkt, path_key)
    Router-->>PeerActor: ProcessedPacket(payload, NextPacket)

    PeerActor->>Router: DecryptBlindedHopData(path_key, encrypted_data)
    Router-->>PeerActor: BlindedRouteData(next_node_id | scid)

    alt next hop by node ID
        PeerActor->>PeerActor: use next_node_id directly
    else next hop by SCID
        PeerActor->>Resolver: RemotePubFromSCID(scid)
        Resolver->>Graph: FetchChannelEdgesByID (on cache miss)
        Graph-->>Resolver: ChannelEdge
        Resolver-->>PeerActor: next pubkey
    end

    PeerActor->>Router: NextEphemeral(path_key)
    Router-->>PeerActor: next_path_key

    PeerActor->>Sender: SendToPeer(next_pubkey, OnionMessage(next_path_key, NextPacket))
    Sender->>Next: OnionMessage (forwarded)

    PeerActor->>Sub: SendUpdate(OnionMessageUpdate)
```

Tags: #diagram #architecture #lnd #onion-messages #networking #skip-lint

## References
- Prose: [Onion Message Forwarding Flow](202603061000-onion-message-forwarding-flow.md)

## Backlinks
- [Onion Message Forwarding Flow](zk/202603061000-onion-message-forwarding-flow.md)
