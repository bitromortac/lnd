# Brontide Noise Protocol Secures Connections

Before any Lightning Network protocol messages are exchanged between peers,
their connection must be cryptographically secured using the Brontide Noise
Protocol. The `brontide` component within the [Peer Network](202603181006-peer-network-management.md)
acts as an authenticated encryption framework, providing a robust handshake
process based on the Noise Protocol Framework (specifically, `Noise_XK`).

When a node connects to another node, it initiates the Brontide handshake. This
handshake mutually authenticates both nodes using their long-term identity
public keys while simultaneously generating ephemeral session keys through
Elliptic Curve Diffie-Hellman (ECDH) operations. Once the handshake is complete,
all subsequent messages are encrypted with a stream cipher (AEAD), ensuring the
confidentiality and integrity of the communication channel. This layer obscures
traffic and prevents active or passive eavesdropping.

Tags: #architecture #security #networking

## References
- Secures: [Peer Network](202603181006-peer-network-management.md)

## Backlinks
- [Lnd Architecture](202603181000-Lnd-Architecture.md)
