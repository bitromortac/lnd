# Invoice Table Extension Adds BOLT 12 Columns

The existing `invoices` table is largely protocol-agnostic — payment hash,
preimage, state machine, amount, and expiry all apply equally to BOLT 12. Rather
than introducing a companion table with a 1:1 JOIN, the table will be widened
with nullable columns for the BOLT 12-specific fields. This is the right
trade-off because BOLT 12 will be the dominant invoice type going forward and
every read would pay the JOIN cost otherwise.

## New Columns

**`is_bolt12`** (`BOOLEAN NOT NULL DEFAULT FALSE`) — Type discriminator
following the existing pattern of `is_amp`, `is_hodl`, and `is_keysend`. Allows
queries to filter by invoice protocol without inspecting other fields.

**`offer_id`** (`BIGINT REFERENCES offers(id)`) — Foreign key to the [offer
store](202603261045-offer-store.md) that spawned this invoice. NULL for BOLT 11
invoices and for BOLT 12 invoices not tied to an offer (e.g., spontaneous
invoice requests). Enables the receiver to trace how many invoices a given offer
has generated and enforce quantity limits.

**`invoice_node_id`** (`BLOB`) — The 33-byte compressed pubkey that signed the
BOLT 12 invoice (`invoice_node_id` TLV type 176). For invoices the node created,
this is its own identity; for received invoices stored on the sender side, this
identifies the payee. NULL for BOLT 11 where the signing key is implicit from
the node identity.

**`invreq_payer_id`** (`BLOB`) — The 33-byte compressed pubkey from the
`invreq_payer_id` TLV (type 88) in the invoice request. This is the payer's
ephemeral key that enables proof of payer — the payer can later prove it was the
entity that requested this invoice. NULL for BOLT 11.

## Reused Columns

Several existing columns map directly to BOLT 12 concepts without modification:

- **`hash`** / **`preimage`** — `invoice_payment_hash` (TLV 168), same
  semantics.
- **`amount_msat`** — `invoice_amount` (TLV 170).
- **`expiry`** — `invoice_relative_expiry` (TLV 166), defaults to 7200 seconds.
- **`created_at`** — `invoice_created_at` (TLV 164).
- **`payment_addr`** — Stores the `path_id` from the blinded path's final hop
  encrypted data. Same lookup path as BOLT 11's payment secret, as documented in
  [Path ID reuses payment address for blinded invoice
  lookup](202603261115-blinded-path-id-replay-prevention.md).
- **`payment_request`** — The authoritative source for the full BOLT 12 invoice.
  Stores the `lni`-prefixed bech32 encoded string containing the complete
  serialized TLV, same column that holds `lnbc` for BOLT 11. The structured
  columns above (`invoice_node_id`, `invreq_payer_id`, etc.) are queryable
  projections extracted at write time — not the source of truth. To reconstruct
  the full invoice (e.g., for signature re-derivation), decode
  `payment_request`. This avoids storing every mirrored field from the offer and
  invoice request individually, which would expand the attack surface for DoS
  via bloated requests.
- **`cltv_delta`** — Already nullable with a comment noting BOLT 12 sets it to
  NULL (CLTV deltas live in `blinded_payinfo` instead).
- **`memo`** — Can hold `offer_description`.
- **`state`** / **`settle_index`** / **`settled_at`** — Protocol agnostic state
  machine fields.

## Not Stored as Separate Columns

- **Signature** — Verified on receipt, then discarded. Recoverable from
  `payment_request` if needed.
- **Merkle tree** — Verification structure, recomputable from `payment_request`.
- **`invoice_paths`** (blinded payment paths) — Ephemeral from the receiver's
  perspective. Recoverable from `payment_request` if needed.
- **`invreq_metadata`**, **`invreq_chain`**, **`invreq_amount`**,
  **`invreq_features`**, **`invreq_quantity`**, **`invreq_payer_note`**,
  **`invreq_paths`**, **`invreq_bip_353_name`** — All mirrored invoice request
  fields live inside the serialized TLV in `payment_request`. Storing them as
  individual columns would expand the attack surface for DoS via bloated
  requests without adding query value.

Tags: #bolt-12 #lnd #storage #invoices #feature-request

## References
- Current schema: [Invoice Database Storage Persists Payment Intentions](lnd/202603250839-invoice-database-storage.md)
- Offer FK target: [Offer Store Persists Long-Lived BOLT 12 Offers](202603261045-offer-store.md)
- Path ID handling: [Path ID Reuses Payment Address for Blinded Invoice Lookup](202603261115-blinded-path-id-replay-prevention.md)
- Invoice data model: [Invoice Data Model Encapsulates Payment Constraints](lnd/202603250831-invoice-data-model.md)
- BOLT 12 TLV fields: [BOLT 12 TLV Message Structures Define Wire Formats](bolts/202603251250-bolt-12-tlv-message-structures.md)

## Backlinks
- [Feature Backlog](202603251500-Feature-Backlog.md)
- [Bolt12 Mvp Direct Peers](202603261230-bolt12-mvp-direct-peers.md)
- [Bolt12 Micro Mvp](202603261245-bolt12-micro-mvp.md)
- [Bolt12 Implementation Strategy](202603261300-bolt12-implementation-strategy.md)
- [Bolt12 Rpc Response Extensions](202603261330-bolt12-rpc-response-extensions.md)
