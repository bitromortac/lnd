# Invoice Acceptor Enables Programmatic Gatekeeping

The Invoice Acceptor operates as an interceptor for incoming payments, providing
an application-level callback to evaluate an incoming Hash Time-Locked Contract
(HTLC) before the node commits to processing it. Normally, when an incoming HTLC
aligns with a registered invoice, the node automatically verifies the
cryptographic constraints and transitions the invoice state.

By registering an Invoice Htlc Modifier via RPC, a user delegates the acceptance
logic to an external application. When a valid HTLC arrives for a watched
invoice, the node pauses processing and streams the HTLC details to the
application. The application acts as a gatekeeper, dynamically analyzing the
payment and responding with an instruction to either accept, settle, or cancel
the HTLC.

This programmatic control enables advanced use cases where a node must enforce
dynamic routing limits, custom user authentication, or multi-party approvals
before allowing an [invoice settlement flow](202603250837-invoice-settlement-flow.md)
to proceed. By deferring the decision outside the core protocol loop, the node
maintains flexibility without altering the underlying consensus rules.

Tags: #invoices #rpc #htlc

## References
- Overrides default validation in: [Invoice registry](202603250838-invoice-registry.md)

## Backlinks
- [Invoices](202603250830-Invoices.md)
- [Invoice Registry](202603250838-invoice-registry.md)
