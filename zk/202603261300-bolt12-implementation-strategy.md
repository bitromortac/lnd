# BOLT 12 Implementation Strategy Builds Bottom-Up

This zettel defines the implementation order for BOLT 12 in LND. Each layer is
independently testable before the next begins. The strategy starts with pure
library code (no LND integration), adds RPCs, then storage and receiver logic,
and finally payment dispatch. The three milestones — [Protocol
Codec](202603261245-bolt12-micro-mvp.md), [Direct
Payment](202603261230-bolt12-mvp-direct-peers.md), and [Routed Payment](202603261030-bolt-12-offer-to-payment-flow.md)
— emerge naturally from this layering.

## Layer 1: Codec and Crypto

Pure library code in a new `bolt12` package. No RPC, no storage, no LND
integration. Testable against the spec's test vectors in `bolts/bolt12/`.

**1a. BOLT 12 bech32** — `lno`/`lnr`/`lni` encoding and decoding per the [bech32
encoding rules](bolts/202603251260-bolt-12-encoding-and-bech32-usage.md). No
checksum (unlike BOLT 11), `+` continuation character support. Test against
`format-string-test.json`.

**1b. TLV message structs** — Go structs for `Offer`, `InvoiceRequest`, and
`Invoice` with TLV serialize/deserialize per the [TLV message
structures](bolts/202603251250-bolt-12-tlv-message-structures.md). Test against
`offers-test.json`.

**1c. Merkle tree and BIP-340 signatures** — tree construction from TLV fields,
tagged hashing, sign/verify per the [signature calculation](bolts/202603251350-bolt-12-signature-calculation.md)
and [Merkle tree spec](bolts/202603251240-bolt-12-merkle-tree-signatures.md).
Test against `signature-test.json`.

**1d. Validation logic** — reader/writer requirement checks: missing mandatory
fields, field matching between invreq and invoice, expiry enforcement, feature
bit handling. Unit tests against spec edge cases.

At this point the codec can decode any `lno1...` string from CLN or other
implementations and verify signatures — no LND wiring yet.

## Layer 2: Protocol Codec RPCs

First LND integration. Sender-side only, no storage, no receiver logic.
Completes the [Protocol Codec](202603261245-bolt12-micro-mvp.md) milestone.

**2a. `DecodeOffer` RPC** — proto definition, wire up the codec. Stateless, no
storage dependencies.

**2b. Single-hop reply path** — trivial builder for the direct-peer case. Sender
is both introduction node and destination.

**2c. Invoice request construction** — ephemeral keypair generation, invreq
signing, onion message wrapping via the existing `SendOnionMessage` path.

**2d. Invoice validation** — byte-for-byte field comparison (types 0–159)
against the original request, [signature
verification](bolts/202603251350-bolt-12-signature-calculation.md),
`invoice_node_id` check.

**2e. `RequestInvoice` RPC** — ties 2b–2d together. Send invreq to a direct
peer, receive invoice via `SubscribeOnionMessages`, validate, return the decoded
invoice. Test against a CLN receiver for interop.

## Layer 3: Receiver Side

Storage and auto-response logic. After this layer, two LND nodes can complete
the offer → invreq → invoice negotiation but not yet settle a payment.

**3a. [Offer store](202603261045-offer-store.md) migration** — new SQL table,
CRUD operations, sqlc queries.

**3b. `CreateOffer` RPC** — generates offer TLV with `offer_issuer_id`, stores
in offer store, returns `lno1...`.

**3c. Invoice request handler** — onion message listener that matches incoming
requests against the offer store and validates per the [invoice request reader
requirements](bolts/202603251330-bolt-12-invoice-request-reader-requirements.md).

**3d. Invoice generation** — preimage and payment hash creation, single-hop
blinded payment path with [`path_id`](202603261115-blinded-path-id-replay-prevention.md)
for replay prevention, invreq field mirroring (types 0–159), Merkle tree
signing.

**3e. [Invoice table extension](202603261130-bolt12-invoice-table-extension.md)
migration** — `is_bolt12`, `offer_id`, `invoice_node_id`, `invreq_payer_id`
columns. `payment_request` holds the `lni1...` string as the [authoritative
source](202603261130-bolt12-invoice-table-extension.md).

**3f. Invoice registration** — store the BOLT 12 invoice, register in the
invoice registry with `path_id` as `payment_addr`.

**3g. Auto-reply** — send the signed invoice back to the sender via the reply
path.

## Layer 4: Payment Dispatch

Adds HTLC settlement. Completes the [Direct Payment](202603261230-bolt12-mvp-direct-peers.md)
milestone.

**4a. [Invoice request store](202603261200-invoice-request-store.md) migration**
— sender persists request blob and ephemeral private key for [proof of
payer](202603261145-proof-of-payer.md).

**4b. `PayOffer` RPC** — extends `RequestInvoice` with HTLC dispatch. Hands
blinded payment paths and payment hash to the router.

**4c. Settlement verification** — HTLC arrives at receiver, invoice registry
matches `hash + path_id`, preimage revealed. Should work with existing HTLC
switch — no new code expected, just integration testing.

**4d. End-to-end itest** — two LND nodes, direct peers, full offer-to-settlement
flow. Verify proof of payer chain (preimage
+ receiver-signed invoice + payer-signed invreq + ephemeral key).

## Layer 5: Routed Payment

Multi-hop support. Completes the [Routed Payment](202603261030-bolt-12-offer-to-payment-flow.md)
milestone.

**5a. [Blinded message path builder](202603261215-reply-message-path-builder.md)**
— production multi-hop builder for reply paths and `offer_paths`.

**5b. [Onion message pathfinding](202603261220-onion-message-pathfinding.md)** —
graph-based route finding to introduction nodes with connectivity and feature
bit constraints.

**5c. Multi-hop blinded payment paths** — extend invoice generation to construct
multi-hop paths via the pathfinding router.

**5d. `offer_paths` support** — receiver privacy without `offer_issuer_id`.
Offers include blinded message paths for receiving invoice requests.

## Interoperability Testing

See [BOLT 12 Interoperability Testing Against CLN and Eclair](202603261315-bolt12-interop-testing.md)
for the full testing strategy. Each layer above has a corresponding interop
phase, from spec test vectors (Layer 1) through mixed-network routed payments
(Layer 5).

Tags: #bolt-12 #lnd #feature-request #workflow

## References
- Protocol Codec milestone: [Protocol Codec Establishes the BOLT 12 Foundation](202603261245-bolt12-micro-mvp.md)
- Direct Payment milestone: [Direct Payment Delivers First BOLT 12 Settlement](202603261230-bolt12-mvp-direct-peers.md)
- Routed Payment milestone: [Routed Payment Completes the BOLT 12 Multi-Hop Flow](202603261030-bolt-12-offer-to-payment-flow.md)
- Feature backlog: [BOLT 12 Feature Backlog](202603251500-Feature-Backlog.md)

## Backlinks
- [Feature Backlog](202603251500-Feature-Backlog.md)
- [Bolt12 Interop Testing](202603261315-bolt12-interop-testing.md)
