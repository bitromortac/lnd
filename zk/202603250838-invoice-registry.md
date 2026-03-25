# Invoice Registry Coordinates Incoming HTLCs

The Invoice Registry acts as an in-memory coordinator that synchronizes arriving
Hash Time-Locked Contract (HTLC) events with their corresponding stored
invoices. Since HTLCs arrive concurrently from multiple channels and payment
paths, the registry provides the crucial concurrency control necessary to
prevent race conditions during payment resolution.

When a channel link receives an HTLC, it queries the registry to determine if
the payment satisfies an open invoice. The registry fetches the expected
parameters from the [invoice database storage](202603250839-invoice-database-storage.md)
and rigorously verifies the incoming parameters. It checks that the payment hash
matches, that the CLTV delay provides a sufficient safety margin, and that the
amount meets the minimum required by the invoice.

If the validation succeeds, the registry records the incoming HTLC and triggers
a state update, orchestrating the transition to `Accepted` or directly to
`Settled`. Crucially, it manages the lifecycle of these incoming events, acting
as the central dispatcher that routes notifications to all external [invoice
subscriptions](202603250840-invoice-subscriptions.md) whenever a watched invoice
changes state.

Tags: #invoices #architecture #htlc #lightning-network

## References
- Drives state changes dictated by: [Invoice state machine](202603250832-invoice-state-machine.md)
- Defers logic to: [Invoice acceptor](202603250836-invoice-acceptor.md)

## Backlinks
- [Invoices](202603250830-Invoices.md)
- [Invoice Data Model](202603250831-invoice-data-model.md)
- [Invoice Creation Flow](202603250833-invoice-creation-flow.md)
- [Invoice Acceptor](202603250836-invoice-acceptor.md)
- [Invoice Settlement Flow](202603250837-invoice-settlement-flow.md)
- [Invoice Database Storage](202603250839-invoice-database-storage.md)
- [Invoice Subscriptions](202603250840-invoice-subscriptions.md)
- [Amp Invoices](202603250841-amp-invoices.md)
