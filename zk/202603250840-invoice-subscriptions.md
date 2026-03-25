# Invoice Subscriptions Stream State Changes Real-Time

To provide responsive applications without excessive polling, nodes expose
invoice subscriptions via RPC. These subscriptions establish a unidirectional
stream from the server to the client that broadcasts updates whenever an invoice
progresses through the [invoice state
machine](202603250832-invoice-state-machine.md).

When a client initiates a subscription, it can choose to track all invoices
globally or narrow its focus to a single, specific invoice identified by its
payment hash. The [invoice registry](202603250838-invoice-registry.md) manages
these active listeners in-memory, acting as an event dispatcher. When an
incoming Hash Time-Locked Contract (HTLC) causes an invoice to transition to
`Accepted`, `Settled`, or `Canceled`, the registry immediately pushes the
updated invoice data to the connected subscribers.

This real-time capability is crucial for implementing [hold
invoices](202603250834-hold-invoices.md), where external systems must react
instantly when funds are locked. The subscription model enables point-of-sale
systems or web applications to confidently dispatch a product or confirm an
order the moment they receive cryptographically secure confirmation of the
settlement.

Tags: #invoices #rpc #architecture #state-machine

## References

## Backlinks
- [Invoices](202603250830-Invoices.md)
- [Hold Invoices](202603250834-hold-invoices.md)
- [Invoice Settlement Flow](202603250837-invoice-settlement-flow.md)
- [Invoice Registry](202603250838-invoice-registry.md)
