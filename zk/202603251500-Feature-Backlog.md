# BOLT 12 Feature Backlog

This collection tracks features that are not yet implemented but are worth
considering for the BOLT 12 integration in LND. Each entry captures the intent,
open questions, and relationship to the specification — not implementation
detail.

## Implementation Strategy

- [BOLT 12 implementation strategy builds bottom-up](202603261300-bolt12-implementation-strategy.md)
  — five layers from pure codec through routed payment. Defines the order in
  which backlog items below should be tackled.

## Protocol Flow

- **(A) Protocol Codec:** [BOLT 12 micro-MVP decodes offers and requests
  invoices](202603261245-bolt12-micro-mvp.md)
  - *Context:* Establishes the encoding/decoding and validation layer.
    `DecodeOffer` + `RequestInvoice` RPCs. Sender-only, no storage, no HTLCs.
    Receiver can be CLN for interop testing.
  - *Status:* Design only.

- **(A) Direct Payment:** [BOLT 12 MVP scopes to directly connected peers](202603261230-bolt12-mvp-direct-peers.md)
  - *Context:* First end-to-end BOLT 12 payment over a single hop. Adds offer
    store, invoice extension, payment dispatch. Defers `offer_paths`, multi-hop
    paths, refund flow.
  - *Status:* Design only. Builds on Protocol Codec.

- **(B) Routed Payment:** [BOLT 12 offer-to-payment flow between two LND nodes](202603261030-bolt-12-offer-to-payment-flow.md)
  - *Context:* Full multi-hop flow with onion message pathfinding, blinded
    message and payment paths, `offer_paths` for receiver privacy.
  - *Status:* Design only. Builds on Direct Payment.

## Codec

- **(A) Offer encode/decode** — serialize and parse the `offer` TLV stream per
  [TLV message structures](bolts/202603251250-bolt-12-tlv-message-structures.md)
  and [bech32 encoding](bolts/202603251260-bolt-12-encoding-and-bech32-usage.md)
  (`lno` prefix, no checksum). Prerequisite for micro-MVP.

- **(A) Invoice request encode/decode** — serialize and parse the
  `invoice_request` TLV stream including payer's [BIP-340
  signature](bolts/202603251350-bolt-12-signature-calculation.md). Prerequisite
  for micro-MVP.

- **(A) Invoice encode/decode** — serialize and parse the `invoice` TLV stream
  including receiver's signature and mirrored fields. Prerequisite for
  micro-MVP.

- **(A) Invoice error encode/decode** — serialize and parse the `invoice_error`
  TLV (onion message namespace type 68). No bech32 encoding — only sent over
  onion messages. Allows the receiver to communicate why an invoice request was
  rejected. See [BOLT 12 Invoice Error
  Requirements](bolts/202603251255-bolt-12-invoice-error-requirements.md).

## Crypto

- **(A) BIP-340 Merkle tree signature library** — creation and verification of
  [BOLT 12 signatures](bolts/202603251350-bolt-12-signature-calculation.md).
  Construct the [Merkle tree](bolts/202603251240-bolt-12-merkle-tree-signatures.md)
  from TLV fields, compute the tagged hash, sign/verify with BIP-340 Schnorr.
  Prerequisite for micro-MVP.

## Storage

- **(A) Offer Store:** [Offer store persists long-lived BOLT 12 offers](202603261045-offer-store.md)
  - *Context:* No BOLT 11 analog — offers are reusable templates that outlive
    any single invoice. Will be a dedicated SQL table with a foreign key from
    BOLT 12 invoices.
  - *Status:* Design only.

- **(A) Invoice Table Extension:** [Invoice table extension adds BOLT 12 columns](202603261130-bolt12-invoice-table-extension.md)
  - *Context:* Widen `invoices` with `is_bolt12`, `offer_id`, `invoice_node_id`,
    `invreq_payer_id`. Most existing columns reuse as-is. No companion table.
  - *Status:* Design only. Depends on offer store (for `offer_id` FK).

- **(A) Invoice Request Store:** [Invoice request store persists outgoing BOLT
  12 requests](202603261200-invoice-request-store.md)
  - *Context:* Sender must store the full invoice request for two reasons:
    spec-mandated byte-for-byte validation of the returned invoice, and proof of
    payer (payer's signature is not mirrored into the invoice). Also stores the
    ephemeral private key for `invreq_payer_id`.
  - *Status:* Design only.

## Onion Messaging

- **(A) Reply Message Path Builder:** [Reply message path builder needed for
  onion message responses](202603261215-reply-message-path-builder.md)
  - *Context:* Payment path builder exists but message paths have no production
    builder — only test helpers. Needed for invoice request reply paths and
    `offer_paths` without `offer_issuer_id`.
  - *Status:* Not implemented. Blocks the invoice request flow.

- **(A) Onion Message Pathfinding:** [Onion message pathfinding to introduction
  nodes](202603261220-onion-message-pathfinding.md)
  - *Context:* Existing pathfinding is payment-oriented (fees, CLTV, liquidity).
    Onion messages need graph-based route finding to introduction nodes with
    only connectivity and feature bit constraints. Required for both sending
    invoice requests and returning invoices.
  - *Status:* Not implemented. Blocks the invoice request flow.

## RPC Surface

### Micro-MVP RPCs

- **(A) `DecodeOffer`** — stateless decode of an `lno1...` string. Returns offer
  fields (description, amount, issuer, chains, expiry, `offer_issuer_id`,
  `offer_paths`). Analogous to `DecodePayReq`.

- **(A) `RequestInvoice` (sender-side)** — takes an offer + peer node ID, sends
  `invoice_request` via onion message, validates the returned invoice, returns
  decoded BOLT 12 invoice. No HTLC dispatch. See
  [micro-MVP](202603261245-bolt12-micro-mvp.md).

### MVP RPCs

- **(A) `CreateOffer`** — receiver creates an offer, stores in [offer
  store](202603261045-offer-store.md), returns `lno1...` encoded string.

- **(A) `PayOffer`** — full sender flow: request invoice + validate
  + dispatch HTLC. Requires [invoice table extension](202603261130-bolt12-invoice-table-extension.md)
    and [invoice request store](202603261200-invoice-request-store.md).

- **(B) `RequestInvoice` (receiver-side):** [RequestInvoice RPC would bridge
  external invoice requests to BOLT 12 invoice generation](202603251505-request-invoice-rpc.md)
  - *Context:* Payee-side RPC for responding to invoice requests. Useful for
    integration testing and out-of-band flows.
  - *Status:* Design only. Depends on offer store.

### Post-MVP RPCs

- **(B) `ListOffers`** — list all offers in the offer store with status (active,
  disabled, expired) and invoice count.

- **(B) `DisableOffer`** — disable an offer without deleting it so in-flight
  requests can still settle.

- **(B) `DecodeInvoice12`** — stateless decode of an `lni1...` string. Returns
  BOLT 12 invoice fields.

- **(C) Offer-less invoice requests** — the spec allows `invoice_request`
  messages that embed offer fields directly without referencing a stored offer
  (e.g., spontaneous "pay-to-me" links). Requires a receiver-side policy for
  accepting offerless requests (opt-in, amount limits) and handling a NULL
  `offer_id` on the invoice. See [BOLT 12 Invoice Request Message Links Payer to
  Offer](bolts/202603251305-bolt-12-invoice-request-links-payer-to-offer.md).

- **(D) `AddInvoice12` + `PayInvoice12`** — create a standalone BOLT 12 invoice
  without an offer or invoice request (like `AddInvoice` for BOLT 11 but with
  blinded paths). No negotiation, no proof of payer — just a one-shot `lni1...`
  string. Requires a corresponding `PayInvoice12` on the sender side that
  accepts an `lni1...` string directly, skipping the offer/invreq flow and the
  byte-for-byte request validation. Use cases: static payment codes, POS
  terminals, interop with BOLT 11 workflows.

### Existing RPCs — Compatibility

See [Existing RPCs Need BOLT 12 Fields in Responses](202603261330-bolt12-rpc-response-extensions.md)
for full detail. Summary:

**Extended with BOLT 12 fields** (`is_bolt12`, `offer_id`, `invoice_node_id`,
`invreq_payer_id`):
- `ListInvoices` / `LookupInvoice` / `SubscribeInvoices` — `Invoice` response
  gains BOLT 12 identifiers and `is_bolt12` filter support.
- `ListPayments` / `TrackPaymentV2` — `Payment` response gains offer and payer
  identity fields.
- `InvoiceAcceptor` — HTLC stream gains BOLT 12 context so external applications
  can apply offer-aware policy.

**Not extended** (separate BOLT 12 RPCs instead):
- `SendPaymentV2` → use `PayOffer`
- `DecodePayReq` → use `DecodeOffer` / `DecodeInvoice12`
- `AddInvoice` → use `CreateOffer` (invoices generated automatically from
  incoming requests)

Tags: #entry-point #bolt-12 #lnd #backlog #feature-request

## References
- Specification: [BOLT 12 Negotiation Protocol Drives Offers and Invoices](bolts/202603251215-Bolt-12-offers-protocol.md)
- Existing invoices: [Invoices Orchestrate Payment Receipt](lnd/202603250830-Invoices.md)

## Backlinks
- [Bolt12 Implementation Strategy](202603261300-bolt12-implementation-strategy.md)
