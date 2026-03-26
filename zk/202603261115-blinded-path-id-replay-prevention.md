# Path ID Reuses Payment Address for Blinded Invoice Lookup

When a receiver constructs a blinded path for a BOLT 12 invoice, it embeds a
`path_id` in the encrypted data of the final hop. This 32-byte value acts as a
unique identifier that ties the blinded path to a specific invoice. When the
HTLC arrives and the receiver decrypts the final hop payload, the `path_id` is
recovered and used to locate the correct invoice — preventing replay of blinded
paths against unrelated payments.

LND already maps this into the existing `payment_addr` column in the `invoices`
table. The invoice registry's update logic checks whether a `pathID` is present
on the incoming HTLC payload and, if so, uses
`InvoiceRefByHashAndAddr(hash, pathID)` to look up the invoice — the same
codepath that handles BOLT 11's `payment_addr` for MPP correlation. When no MPP
record is present but a `path_id` is, the bytes are copied directly into the
payment address field for storage and matching.

This dual use means `payment_addr` carries different semantics depending on
invoice type: for BOLT 11 it is a random probing deterrent shared with the payer
in the invoice encoding, while for BOLT 12 it is a receiver-chosen secret that
never leaves the encrypted blob. The lookup mechanism is identical in both cases
— `(hash, addr)` — so no schema change is needed. The distinction is purely in
how the value originates and who knows it.

Tags: #bolt-12 #lnd #invoices #blinded-paths

## References
- Lookup logic: [Invoice Registry Coordinates Incoming HTLCs](lnd/202603250838-invoice-registry.md)
- Blinded path construction: [Blinded Paths Obscure Payment Destinations](lnd/202603181015-blinded-paths-privacy.md)
- Invoice data model: [Invoice Data Model Encapsulates Payment Constraints](lnd/202603250831-invoice-data-model.md)
- Flow context: [BOLT 12 Offer-to-Payment Flow Between Two LND Nodes](202603261030-bolt-12-offer-to-payment-flow.md)

## Backlinks
- [Bolt 12 Offer To Payment Flow](202603261030-bolt-12-offer-to-payment-flow.md)
- [Bolt12 Invoice Table Extension](202603261130-bolt12-invoice-table-extension.md)
- [Bolt12 Mvp Direct Peers](202603261230-bolt12-mvp-direct-peers.md)
- [Bolt12 Implementation Strategy](202603261300-bolt12-implementation-strategy.md)
