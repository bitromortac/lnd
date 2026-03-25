# Watchtower Client Outsources Monitoring Responsibilities

The watchtower client protects the local Lightning node by outsourcing the
responsibility of monitoring the blockchain to remote third parties. It operates
by constantly backing up the latest channel state.

When the channel state is updated, the client creates a "justice transaction"
that spends the revoked outputs. It encrypts this transaction using a key
derived from the revoked commitment transaction's txid. This encrypted blob is
then forwarded to the [watchtower
server](202603251002-watchtower-server-enforces-breaches.md) along with a
"breach hint" (a prefix of the txid). The client can safely go offline, knowing
that the tower cannot decipher the channel's value or participants unless an
actual breach occurs.

Tags: #architecture #security

## References
- Parent architecture: [Watchtower Architecture](202603251000-Watchtower-Architecture.md)

## Backlinks
- [Watchtower Architecture](202603251000-Watchtower-Architecture.md)
- [Watchtower Server Enforces Breaches](202603251002-watchtower-server-enforces-breaches.md)
