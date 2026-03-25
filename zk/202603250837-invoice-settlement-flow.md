# Invoice Settlement Flow Finalizes Payment Intent

Settling an invoice is the irreversible action that transitions the contract to
its terminal state, allowing the node to claim the associated funds. When an
incoming Hash Time-Locked Contract (HTLC) arrives that satisfies the invoice's
value and CLTV constraints, the node accepts it. However, the transaction is not
finalized until the node reveals the cryptographic preimage corresponding to the
invoice's payment hash.

The settlement process requires the node to first verify that the invoice is not
already settled or canceled, respecting the [invoice state
machine](202603250832-invoice-state-machine.md). The node then extracts the
stored preimage and publishes an update to the channel state, transmitting the
preimage backward along the payment route. This action definitively settles the
HTLC, cryptographically proving receipt to the payer.

Simultaneously, the settlement flow updates the [invoice database storage](202603250839-invoice-database-storage.md)
to record the terminal `Settled` state along with the amount paid and the
settlement time. Any external clients monitoring the invoice via [invoice
subscriptions](202603250840-invoice-subscriptions.md) receive a notification of
the transition, allowing the application layer to deliver the purchased goods or
services.

Tags: #invoices #settlement #htlc #architecture

## References
- Bypassed temporarily by: [Hold invoices](202603250834-hold-invoices.md)
- Requires validation by: [Invoice registry](202603250838-invoice-registry.md)

## Backlinks
- [Invoices](202603250830-Invoices.md)
- [Invoice State Machine](202603250832-invoice-state-machine.md)
- [Hold Invoices](202603250834-hold-invoices.md)
- [Invoice Acceptor](202603250836-invoice-acceptor.md)
- [Invoice Database Storage](202603250839-invoice-database-storage.md)
- [Amp Invoices](202603250841-amp-invoices.md)
