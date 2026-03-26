# Invoice Request Store Persists Outgoing BOLT 12 Requests

The sender must store the full `invoice_request` it sends to a receiver. This is
required for two distinct reasons: invoice validation at receipt time, and proof
of payer after settlement.

## Invoice Validation Requires the Original Request

The [invoice reader requirements](bolts/202603251340-bolt-12-invoice-reader-requirements.md)
mandate that the sender reject any invoice whose fields in ranges 0–159 and
1000000000–2999999999 do not exactly match the original invoice request. This
byte-for-byte comparison ensures the receiver did not tamper with the offer
terms, amount, chain, or payer identity. Additional checks depend on state from
the request: the sender must verify `invoice_node_id` against `offer_issuer_id`
or the `blinded_node_id` it originally sent to, and confirm the invoice arrived
via the request's `reply_path`. None of this is possible without the original
invoice request in hand.

## Proof of Payer Requires the Payer's Signature

The payer's BIP-340 signature on the invoice request (TLV type 240) is not
mirrored into the invoice — only fields 0–159 are. Without the original request,
the [proof of payer](202603261145-proof-of-payer.md) chain is incomplete: the
receiver could fabricate an invoice with any `invreq_payer_id` it chooses. The
payer's signature on the request is what proves the key holder actually
initiated it.

## Storage Design

The invoice request should be stored as a serialized blob alongside the payment,
similar to how `payment_intents.intent_payload` stores the invoice. The
ephemeral private key for `invreq_payer_id` must be stored separately since it
never appears in any wire message. Together, the sender needs three pieces of
BOLT 12 state per payment:

1. **Invoice request blob** — for validation and proof of payer
2. **Invoice blob** — in `intent_payload`, the authoritative source
3. **Ephemeral private key** — for demonstrating control of `invreq_payer_id`

Tags: #bolt-12 #lnd #storage #feature-request

## References
- Validation rules: [BOLT 12 Invoice Reader Requirements](bolts/202603251340-bolt-12-invoice-reader-requirements.md)
- Proof of payer: [Proof of Payer Binds a Payment to the Entity That Requested
  It](202603261145-proof-of-payer.md)
- Sender storage: [Sender-Side Payment Storage Mostly Supports BOLT 12](202603261100-sender-side-bolt12-storage-ready.md)
- Flow context: [BOLT 12 Offer-to-Payment Flow Between Two LND Nodes](202603261030-bolt-12-offer-to-payment-flow.md)

## Backlinks
- [Feature Backlog](202603251500-Feature-Backlog.md)
- [Bolt 12 Offer To Payment Flow](202603261030-bolt-12-offer-to-payment-flow.md)
- [Sender Side Bolt12 Storage Ready](202603261100-sender-side-bolt12-storage-ready.md)
- [Proof Of Payer](202603261145-proof-of-payer.md)
- [Bolt12 Mvp Direct Peers](202603261230-bolt12-mvp-direct-peers.md)
- [Bolt12 Micro Mvp](202603261245-bolt12-micro-mvp.md)
- [Bolt12 Implementation Strategy](202603261300-bolt12-implementation-strategy.md)
