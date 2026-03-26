# Existing RPCs Need BOLT 12 Fields in Responses

Several existing RPCs return invoice or payment data that will include BOLT 12
entries once the new invoice type is supported. These RPCs do not need new
endpoints — they already work — but their response messages must be extended
with BOLT 12 identifiers so that callers can fetch additional context (offer
details, invoice request data, payer identity).

## Invoice Response Extensions

The `Invoice` proto message is returned by `ListInvoices`, `LookupInvoice`, and
`SubscribeInvoices`. It currently carries BOLT 11 fields. For BOLT 12 invoices
the following fields should be added:

- **`is_bolt12`** (`bool`) — type discriminator so callers can branch without
  inspecting `payment_request` prefixes.
- **`offer_id`** (`int64`) — FK to the offer store. Allows callers to fetch the
  originating offer via a future `GetOffer` RPC for full offer details
  (description, amount policy, issuer, quantity limits).
- **`invoice_node_id`** (`bytes`) — the 33-byte pubkey that signed the invoice.
  For self-created invoices this is the node's own identity; for received
  invoices it identifies the payee.
- **`invreq_payer_id`** (`bytes`) — the 33-byte ephemeral pubkey from the
  invoice request. Identifies who requested this invoice for [proof of payer](202603261145-proof-of-payer.md)
  purposes.

The `payment_request` field already holds the `lni1...` string for BOLT 12
invoices — no change needed there.

## Payment Response Extensions

The `Payment` proto message is returned by `ListPayments` and streamed by
`TrackPaymentV2`. For BOLT 12 payments:

- **`offer`** (`string`) — the `lno1...` encoded offer that initiated this
  payment. Recoverable from the invoice in `payment_intents.intent_payload`
  (offer fields are mirrored).
- **`invreq_payer_id`** (`bytes`) — the sender's ephemeral pubkey, so the caller
  can identify which payments used which payer identity.

The `payment_request` field will hold the `lni1...` invoice string.

## Invoice Acceptor Extensions

The `InvoiceAcceptor` (HTLC modifier) streams HTLC details to external
applications for accept/settle/cancel decisions. For BOLT 12 HTLCs, the stream
should include:

- **`is_bolt12`** (`bool`) — so the acceptor logic can apply BOLT 12-specific
  policy.
- **`offer_id`** (`int64`) — so the application can look up the offer context.
- **`invreq_payer_id`** (`bytes`) — so the application knows who requested this
  invoice (e.g., for allowlisting known payers).

## Subscribe Invoices

`SubscribeInvoices` already works for BOLT 12 — the invoice registry dispatches
state changes regardless of invoice type. The `Invoice` response extensions
above automatically flow through to subscribers. Should support filtering by
`is_bolt12` to let callers subscribe only to BOLT 12 invoice events.

## No Changes Needed

These existing subsystems work as-is for BOLT 12:

- **Invoice registry HTLC validation** — the `pathID` codepath in `updateMpp`
  already handles BOLT 12 payment address matching, amount checks, and state
  transitions.
- **Invoice state machine** — same states (`Open`, `Accepted`, `Settled`,
  `Canceled`), same transitions.
- **`ValidateInvoice`** — memo size, payment request size, feature vector checks
  all apply. May need a minor tweak to `requiresPreimage` for BOLT 12 hold
  invoice support.
- **`TrackPaymentV2`** — if `PayOffer` uses the standard payment lifecycle
  internally, payment tracking notifications come for free.

Tags: #bolt-12 #lnd #rpc #invoices #feature-request

## References
- Invoice subscriptions: [Invoice Subscriptions Stream State Changes Real-Time](lnd/202603250840-invoice-subscriptions.md)
- Invoice acceptor: [Invoice Acceptor Enables Programmatic Gatekeeping](lnd/202603250836-invoice-acceptor.md)
- Invoice registry: [Invoice Registry Coordinates Incoming HTLCs](lnd/202603250838-invoice-registry.md)
- Proof of payer: [Proof of Payer Binds a Payment to the Entity That Requested
  It](202603261145-proof-of-payer.md)
- Invoice table extension: [Invoice Table Extension Adds BOLT 12 Columns](202603261130-bolt12-invoice-table-extension.md)

## Backlinks
- [Feature Backlog](202603251500-Feature-Backlog.md)
