# Watchtower Server Enforces Channel Breaches

The watchtower server is a specialized service that monitors the blockchain for
malicious behavior on behalf of offline clients. It runs blindly, storing only
encrypted blobs and "breach hints" without knowledge of the underlying funds or
channel participants.

The server compares every new transaction block against its database of breach
hints. If a counterparty publishes an old, revoked channel state, the resulting
transaction ID matches a stored hint. The server then uses the full transaction
ID to decrypt the corresponding blob, which yields a pre-signed justice
transaction. By immediately broadcasting this justice transaction, the tower
punishes the cheater by sweeping all the funds, fulfilling its contract with the
[watchtower client](202603251001-watchtower-client-outsources-monitoring.md).

Tags: #architecture #security #on-chain

## References
- Outsourced by: [Watchtower Client](202603251001-watchtower-client-outsources-monitoring.md)

## Backlinks
- [Watchtower Architecture](202603251000-Watchtower-Architecture.md)
- [Watchtower Client Outsources Monitoring](202603251001-watchtower-client-outsources-monitoring.md)
