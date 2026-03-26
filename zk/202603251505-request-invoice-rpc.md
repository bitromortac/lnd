# RequestInvoice RPC Would Bridge External Invoice Requests

A `RequestInvoice` RPC would accept a raw BOLT 12 `invoice_request` message and
return a fully constructed BOLT 12 `invoice`. This fills the role of the
"invoice request reader" in the [BOLT 12 negotiation protocol](bolts/202603251215-Bolt-12-offers-protocol.md)
— the node receives a request, validates it against a stored offer, and responds
with a signed invoice containing blinded paths and payment parameters.

## Motivation

Today, LND's [invoice creation flow](lnd/202603250833-invoice-creation-flow.md)
is oriented around BOLT 11: a client calls `AddInvoice`, the node generates a
preimage and payment hash, and returns an encoded payment request string. BOLT
12 inverts this — the *payer* initiates by sending an `invoice_request`, and the
*payee* must respond with a purpose-built invoice. There is currently no RPC
surface for this payee-side response.

Beyond production use, an explicit RPC is valuable for integration testing. When
building and verifying the receiver flow in isolation, a test harness can
construct an `invoice_request` externally and call `RequestInvoice` directly —
exercising offer validation, blinded path generation, and invoice signing
without needing a fully wired sender or onion message transport.

## Open Questions

**Should this be an RPC at all?** In the standard BOLT 12 flow, the invoice
request arrives over an onion message and the node responds automatically. An
RPC would be useful when the node wants to delegate invoice generation to an
external application (similar to the [Invoice Acceptor](lnd/202603250836-invoice-acceptor.md)
pattern for inbound HTLCs). The question is whether there is a real use case for
out-of-band invoice request handling, or whether the onion message handler
should simply do this internally.

**Offer lookup and validation.** The [invoice request reader requirements](bolts/202603251330-bolt-12-invoice-request-reader-requirements.md)
mandate that the request's offer fields exactly match a valid, unexpired offer.
This implies the node needs an offer store — a prerequisite feature that
determines whether `RequestInvoice` can look up the offer itself or whether the
caller must supply it.

**Blinded path construction.** The [invoice writer requirements](bolts/202603251335-bolt-12-invoice-writer-requirements.md)
require every BOLT 12 invoice to include `invoice_paths` with blinded payment
routes. Generating these paths requires cooperation with the router and
knowledge of the node's current channel graph. This is non-trivial state that an
RPC caller likely cannot supply, suggesting the node must own this step.

**Hold semantics.** Should `RequestInvoice` support hold-invoice behavior, where
the preimage is not auto-revealed? This could compose with the existing [hold
invoice](lnd/202603250834-hold-invoices.md) machinery, but the interaction
between BOLT 12 signatures and deferred settlement needs careful design.

**Proof of payer.** The `invreq_payer_id` in the request enables cryptographic
proof that a specific entity requested the invoice. The RPC response should
expose this so applications can bind the payer identity to external records.

Tags: #bolt-12 #lnd #rpc #invoices #feature-request

## References
- Spec requirements (reader side): [BOLT 12 Invoice Request Reader Requirements](bolts/202603251330-bolt-12-invoice-request-reader-requirements.md)
- Spec requirements (writer side): [BOLT 12 Invoice Writer Requirements](bolts/202603251335-bolt-12-invoice-writer-requirements.md)
- Existing creation flow: [Invoice Creation Flow](lnd/202603250833-invoice-creation-flow.md)
- Interceptor pattern: [Invoice Acceptor](lnd/202603250836-invoice-acceptor.md)
- BOLT 12 invoice message: [BOLT 12 Invoice Message Provides Final Payment
  Parameters](bolts/202603251310-bolt-12-invoice-message-provides-payment-parameters.md)

## Backlinks
- [Feature Backlog](202603251500-Feature-Backlog.md)
