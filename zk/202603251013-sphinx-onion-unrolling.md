# Sphinx Onion Unrolling Decrypts Routing Packets

The `sphinx` unrolling subsystem within the `lnd` architecture handles the
iterative decryption of routing packets to ensure sender privacy. As a Lightning
Network payment is forwarded across multiple nodes, each hop receives a
fixed-size, heavily encrypted data blob known as the "onion packet".

When the [HTLC Switch](202603181002-htlc-switch-routing.md) receives an HTLC,
it takes the attached onion packet and processes it through the Sphinx decoding
function. The current node uses its private identity key and an ephemeral
Diffie-Hellman key (provided inside the packet) to derive a shared secret. It
then peels off a single layer of the encryption—much like peeling an onion.
This decryption process reveals only the necessary routing instructions for the
current hop, such as the required forward amount, the outgoing channel ID, and
the locktime, as well as the new, slightly smaller onion packet that must be
passed to the next node. Because of this fixed-size packet design, no
intermediate node can deduce its position in the overall path, nor can it
discover the original sender or final recipient.

Tags: #architecture #privacy #routing #cryptography

## References
- Invoked by: [HTLC Switch](202603181002-htlc-switch-routing.md)

## Backlinks
- [Lnd Architecture](202603181000-Lnd-Architecture.md)
