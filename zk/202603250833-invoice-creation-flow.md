# Invoice Creation Flow Establishes Intent to Receive

Creating an invoice serves as a formal declaration by a node that it is
ready to accept funds under specific conditions. When an external client or RPC
user initiates creation, the node generates a new cryptographic preimage if one
was not explicitly provided, and computes the corresponding payment hash to form
the basis of the invoice.

The node persists the newly created invoice to the [database storage](202603250839-invoice-database-storage.md),
ensuring the intent survives process restarts. The creation process then binds
the payment hash, value, expiration, and routing hints into a standardized payment
request string (typically encoded as BOLT-11). This string is distributed to the
payer, conveying all the necessary details to construct a valid route and HTLC.

Critically, the creation step defines the invariant constraints—such as the
minimum CLTV delta and expected amount—that the [invoice registry](202603250838-invoice-registry.md)
must later verify against any incoming payment before transitioning the invoice
to an accepted state.

Tags: #invoices #rpc #lightning-network

## References
- Builds intent based on: [Invoice data model](202603250831-invoice-data-model.md)

## Backlinks
- [Invoices](202603250830-Invoices.md)
- [Invoice Database Storage](202603250839-invoice-database-storage.md)
