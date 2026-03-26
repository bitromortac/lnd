# Onion Message Pathfinding to Introduction Nodes

To send an onion message to a blinded path, the sender must first find a route
through the regular graph from itself to the introduction node of that blinded
path. LND's existing pathfinding in `routing/router.go` is built for payment
routing — it optimizes for fee and CLTV constraints that do not apply to onion
messages. There is currently no pathfinding capability for onion message
delivery.

## Two-Part Route Construction

Sending an onion message to a blinded path involves stitching together two
segments:

1. **Cleartext segment** — a route from the sender to the introduction node,
   found via graph pathfinding. The sender knows the introduction node's
   identity from the blinded path.
2. **Blinded segment** — the introduction node through the encrypted hops to the
   recipient, provided by the blinded path itself.

The sender constructs a single onion packet spanning both segments. The
cleartext hops forward normally; the introduction node transitions into the
blinded segment using the blinding point.

## Differences From Payment Pathfinding

Onion message pathfinding is simpler than payment pathfinding in some ways but
different in others:

- **No fee optimization** — onion messages do not carry value, so there are no
  fee constraints to minimize.
- **No CLTV constraints** — no timelocks to accumulate.
- **No liquidity requirements** — no channel capacity checks needed.
- **Connectivity is king** — the only requirement is that each hop on the route
  has a peer connection to the next. The graph must indicate reachability, not
  capacity.
- **Feature bit filtering** — hops must support onion message forwarding (the
  `onion_messages` feature bit).

## Feature Bits Already in Place

The two feature bits needed for BOLT 12 are already defined and advertised by
LND:

- **`OnionMessagesOptional` (bit 39)** — signals the node can forward onion
  messages. Advertised in `Init` and `NodeAnn`. Can be disabled via
  `NoOnionMessages` config flag. This is the bit that onion message pathfinding
  must filter on: every hop in the cleartext segment must advertise it.
- **`RouteBlindingOptional` (bit 25)** — signals support for blinded payments.
  Relevant for blinded payment path construction (`BuildBlindedPaymentPaths`
  already checks for it) but not required for message-layer pathfinding.

No new feature bits are needed for BOLT 12. The graph already contains the data
to filter eligible hops — the pathfinder just needs to query for nodes
advertising bit 39.

## Where This Is Needed

Every onion message send in the BOLT 12 flow requires this: the sender finding a
path to the offer's `offer_paths` introduction node (Phase 2), and the receiver
finding a path to the reply path's introduction node to send the invoice back
(Phase 3). Without this, the [reply message path builder](202603261215-reply-message-path-builder.md)
has blinded segments but no way to reach them.

Tags: #bolt-12 #lnd #onion-message #routing #feature-request

## References
- Payment pathfinding: [Pathfinding Router Discovers Payment Routes](lnd/202603181005-pathfinding-router.md)
- Reply path builder: [Reply Message Path Builder Needed for Onion Message
  Responses](202603261215-reply-message-path-builder.md)
- Flow context: [BOLT 12 Offer-to-Payment Flow Between Two LND Nodes](202603261030-bolt-12-offer-to-payment-flow.md)

## Backlinks
- [Feature Backlog](202603251500-Feature-Backlog.md)
- [Bolt12 Mvp Direct Peers](202603261230-bolt12-mvp-direct-peers.md)
- [Bolt12 Implementation Strategy](202603261300-bolt12-implementation-strategy.md)
