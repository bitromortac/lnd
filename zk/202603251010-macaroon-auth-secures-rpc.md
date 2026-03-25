# Macaroon Auth Secures RPC

The daemon uses Macaroons, a decentralized authorization token standard, to
secure its internal logic against unauthorized access. This mechanism allows the
[Lnd Daemon Process](202603181001-lnd-daemon-process.md) to enforce
fine-grained, granular permissions for its gRPC and REST APIs.

Unlike basic API keys, macaroons contain specific capabilities or constraints
(e.g., "read-only access", "can only create invoices", or "valid only from this
IP address"). When an external client makes a request to the node's API, the
request is intercepted and the accompanying macaroon is validated. The
authorization system decodes the token, checks the embedded permissions against
the required operations for that endpoint, and verifies the token's
cryptographic signature using the node's secret root key. This strict
enforcement ensures that sensitive operations, like spending funds or modifying
network policies, are tightly restricted to authorized clients.

Tags: #architecture #security #authentication #macaroon #rpc

## References
- Secures: [Daemon Process](202603181001-lnd-daemon-process.md)

## Backlinks
- [Lnd Architecture](202603181000-Lnd-Architecture.md)
