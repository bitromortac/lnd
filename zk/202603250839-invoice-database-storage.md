# Invoice Database Storage Persists Payment Intentions

Invoices represent a critical financial contract that must survive process
restarts and machine crashes. To guarantee durability, invoices are written to a
SQL-backed storage system. This persistence mechanism ensures that an [invoice
creation flow](202603250833-invoice-creation-flow.md) successfully records the
node's intent to receive funds before any payment request is transmitted to a
payer.

Invoices are indexed and retrieved using dual identifiers: the primary payment
hash, and a newer, optional payment address. The payment address serves to
prevent probing attacks and guarantees the payer explicitly requested the newly
created invoice. The storage layer must atomically record updates whenever an
invoice's state changes—such as when it accepts an incoming Hash Time-Locked
Contract (HTLC) or when the invoice is finally settled or canceled.

This atomicity ensures that if the node crashes during the [invoice settlement
flow](202603250837-invoice-settlement-flow.md), it will not accidentally forget
that an invoice was settled or that a preimage was revealed. The persistent
state guarantees the [invoice state machine](202603250832-invoice-state-machine.md)
maintains its integrity across all channel connections over time.

Tags: #invoices #storage #database #architecture

## References
- Queried by: [Invoice registry](202603250838-invoice-registry.md)

## Backlinks
- [Invoices](202603250830-Invoices.md)
- [Invoice Creation Flow](202603250833-invoice-creation-flow.md)
- [Invoice Settlement Flow](202603250837-invoice-settlement-flow.md)
- [Invoice Registry](202603250838-invoice-registry.md)
