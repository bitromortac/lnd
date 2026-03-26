# BOLT 12 Interoperability Testing Against CLN and Eclair

CLN and Eclair both have production BOLT 12 support and serve as interop
targets. Testing aligns with the layers defined in the [implementation
strategy](202603261300-bolt12-implementation-strategy.md), with each layer
introducing a new category of interop verification.

## Spec Test Vectors (Layer 1)

The BOLT 12 spec ships test vectors in `bolts/bolt12/`
(`format-string-test.json`, `offers-test.json`, `signature-test.json`). These
validate the codec in isolation without any network. If our code decodes vectors
produced by other implementations and produces identical encodings, the wire
format is correct.

## Cross-Implementation Decode (Layer 1)

Generate offers and invoices on CLN and Eclair, decode them with our codec, and
vice versa. Catches subtle serialization differences that test vectors might not
cover: field ordering, unknown TLV handling, optional field presence, and
encoding edge cases.

## Negotiation Interop (Layer 2–3)

The Protocol Codec milestone is designed for this. Two direct peers, one LND and
one CLN or Eclair.

**LND as sender:** Our `RequestInvoice` sends an invreq to a CLN or Eclair node
with an existing offer and validates the returned invoice. This exercises our
TLV encoding, signature creation, and invoice validation against a real
implementation.

**LND as receiver** (once Layer 3 is built): CLN's `fetchinvoice` or Eclair's
equivalent sends an invreq to our node. Our handler matches the offer, generates
an invoice, and replies. The other node validates it. This exercises our offer
matching, invoice generation, and Merkle signing.

## Payment Interop (Layer 4)

Full HTLC settlement in both directions:

- **LND pays CLN/Eclair:** `PayOffer` against an offer created on the other
  node. Verifies blinded payment path handling, HTLC dispatch, and preimage
  settlement.
- **CLN/Eclair pays LND:** The other node pays an LND-created offer. Verifies
  our invoice registration, `path_id` matching, and preimage reveal.

Key edge cases to cover:
- MPP vs single-part payment handling
- Blinded path construction differences (padding, dummy hops)
- `invoice_error` messages for rejected requests
- Expiry boundary conditions
- Amount mismatch rejection

## Routed Interop (Layer 5)

Multi-hop payments where LND, CLN, and Eclair nodes form a mixed network. Tests
onion message forwarding across implementations, multi-hop blinded paths with
different padding strategies, and pathfinding to introduction nodes operated by
other implementations.

Tags: #bolt-12 #lnd #testing #interoperability

## References
- Implementation strategy: [BOLT 12 Implementation Strategy Builds Bottom-Up](202603261300-bolt12-implementation-strategy.md)
- Protocol Codec milestone: [Protocol Codec Establishes the BOLT 12 Foundation](202603261245-bolt12-micro-mvp.md)
- Direct Payment milestone: [Direct Payment Delivers First BOLT 12 Settlement](202603261230-bolt12-mvp-direct-peers.md)

## Backlinks
- [Bolt12 Implementation Strategy](202603261300-bolt12-implementation-strategy.md)
