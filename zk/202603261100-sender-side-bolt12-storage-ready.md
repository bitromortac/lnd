# Sender-Side Payment Storage Mostly Supports BOLT 12

The payment schema on the sender (payer) side is already structured to
accommodate most of BOLT 12. Two additions are needed: storage for the invoice
request and the ephemeral private key.

The `payment_intents` table discriminates payment types via `intent_type` (with
BOLT 12 as a planned value) and stores the serialized payload in
`intent_payload`. The schema comments explicitly call out BOLT 12 offer data as
a future payload type. The full invoice can be stored here as the authoritative
source for the receiver's signed response.

Two additional pieces of sender-side state are needed beyond what exists today:

1. **The invoice request** — must be stored as a serialized blob. The [invoice
   reader requirements](bolts/202603251340-bolt-12-invoice-reader-requirements.md)
   mandate byte-for-byte comparison of fields 0–159 against the original request
   at validation time. After settlement, the payer's signature on the request
   (type 240, not mirrored into the invoice) is needed for a complete [proof of
   payer](202603261145-proof-of-payer.md) chain. See [Invoice Request
   Store](202603261200-invoice-request-store.md).

2. **The ephemeral private key** behind `invreq_payer_id` — the payer generates
   a fresh keypair per invoice request. The public key is mirrored into the
   invoice, but the private key never appears in any wire message. Without it,
   the payer cannot demonstrate control of `invreq_payer_id`.

On the routing side, `payment_route_hop_blinded` already stores blinded hop data
— the blinding point for the introduction node, encrypted data per hop, and the
total blinded path amount. Since BOLT 12 invoices mandate `invoice_paths` with
blinded payment routes, the sender's route recording naturally captures these
when the HTLC attempt is persisted. The existing `payment_route_hops` parent
table and its per-hop payload children (`payment_route_hop_mpp`,
`payment_route_hop_amp`, `payment_route_hop_blinded`) follow a pattern where
each payment type gets a child table — BOLT 12 fits cleanly into
`payment_route_hop_blinded` without extension.

Tags: #bolt-12 #lnd #storage

## References
- Sender schema: [Payment system migration](../sqldb/sqlc/migrations/000010_payments.up.sql)
- Receiver side (new work): [Offer Store Persists Long-Lived BOLT 12 Offers](202603261045-offer-store.md)
- Flow context: [BOLT 12 Offer-to-Payment Flow Between Two LND Nodes](202603261030-bolt-12-offer-to-payment-flow.md)

## Backlinks
- [Bolt 12 Offer To Payment Flow](202603261030-bolt-12-offer-to-payment-flow.md)
- [Proof Of Payer](202603261145-proof-of-payer.md)
- [Invoice Request Store](202603261200-invoice-request-store.md)
