# Peer Network Multiplexes Connections

The `peer` and `brontide` subsystems handle the encrypted transport (BOLT 8) for
the [Lnd Architecture](202603181000-Lnd-Architecture.md). They manage the
lifecycle of network connections by orchestrating an authenticated handshake and
then multiplexing distinct control logic and lightning messages over a single
TCP connection per peer.

These components ensure that the node can securely communicate with others over
the Lightning Network, exposing the `MessageConn` and `MessageSender`
interfaces. This allows higher-level subsystems, such as the [HTLC
Switch](202603181002-htlc-switch-routing.md), to transmit serialized messages
(e.g., HTLC adds, settles, or fails) or custom feature sets transparently
without needing to understand the underlying cryptographic transport protocols
or reconnect logic.

Tags: #architecture #networking #protocol #multiplexing

## References
- Multiplexes for: [HTLC Switch](202603181002-htlc-switch-routing.md)

## Backlinks
- [Funding Manager](202603181007-funding-manager.md)
- [Brontide Noise Protocol Handshake](202603251012-brontide-noise-protocol-handshake.md)
