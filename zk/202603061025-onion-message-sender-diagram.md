# Onion Message Sender Flow Diagram

Construction and transmission sequence for an outgoing onion message. See [Onion
Message Sender Flow](202603061020-onion-message-sender-flow.md) for prose
explanation.

```mermaid
sequenceDiagram
    participant Caller as Caller (itest / application)
    participant Sphinx as sphinx library
    participant RPC as rpcServer.SendOnionMessage
    participant Server as server.SendOnionMessage
    participant Peer as brontide.SendMessageLazy
    participant Wire as TCP (to first hop)

    Note over Caller,Sphinx: Caller constructs the onion (outside LND)

    Caller->>Sphinx: BuildBlindedPath(sessionKey, hops)
    Sphinx-->>Caller: BlindedPathInfo{path, BlindingPoint}

    Caller->>Sphinx: OnionMessageBlindedPathToSphinxPath(blindedPath, finalTLVs)
    Sphinx-->>Caller: SphinxPath

    Caller->>Sphinx: NewOnionPacket(sphinxPath, msgSessionKey, assocData=nil)
    Sphinx-->>Caller: OnionPacket (serialised blob)

    Note over Caller,Wire: Caller sends via RPC

    Caller->>RPC: SendOnionMessage{peer, path_key, onion_blob}
    RPC->>Server: SendOnionMessage(ctx, peerPub, pathKey, onion)
    Server->>Server: FindPeerByPubStr(peerPub)
    Server->>Peer: wait ActiveSignal
    Server->>Peer: SendMessageLazy(lowPriority, OnionMessage{pathKey, onion})
    Peer->>Wire: serialised OnionMessage (type 513)
```

Tags: #diagram #architecture #lnd #onion-messages

## References
- Prose: [Onion Message Sender Flow](202603061020-onion-message-sender-flow.md)

## Backlinks
- [Onion Message Sender Flow](zk/202603061020-onion-message-sender-flow.md)
