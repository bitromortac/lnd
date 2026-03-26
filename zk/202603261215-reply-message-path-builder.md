# Reply Message Path Builder Needed for Onion Message Responses

BOLT 12's invoice request flow requires the sender to include a `reply_path` in
the onion message so the receiver can send the invoice back. LND currently has a
production builder for blinded **payment** paths (`BuildBlindedPaymentPaths` in
`routing/blindedpath/blinded_path.go`) but no production builder for blinded
**message** paths. The only message path construction logic lives in
`onionmessage/test_utils.go` as a test helper.

## Payment Paths vs. Message Paths

Both are structurally similar — an introduction node followed by encrypted hops
— but they carry different payloads. Payment paths embed fee policies, CLTV
deltas, and HTLC limits (`blinded_payinfo`) because forwarding nodes need to
know how to price the relay. Message paths carry no fee or CLTV data; they are
pure routing envelopes for onion messages. The `BuildBlindedPaymentPaths`
builder is therefore not directly reusable for message paths.

## Where Reply Paths Are Needed

The sender includes a reply path in the `invoice_request` onion message. The
[invoice reader requirements](bolts/202603251340-bolt-12-invoice-reader-requirements.md)
mandate that the invoice arrive via the request's `onionmsg_tlv` `reply_path`.
Without a reply path, the receiver has no way to send the invoice back to the
sender.

The receiver also needs message paths when constructing `offer_paths` for offers
shared without an `offer_issuer_id` — these allow invoice requests to reach the
receiver without revealing its node identity.

## Existing Infrastructure

The `onionmessage` package can already process (forward and deliver) onion
messages along blinded paths. The `ReplyPath` field exists on `OnionEndpoint`.
What is missing is a builder that selects appropriate routes from the graph and
constructs the `sphinx.BlindedPath` with encrypted hop data suitable for message
relay.

Tags: #bolt-12 #lnd #onion-message #feature-request

## References
- Payment path builder: [Blinded Paths Obscure Payment Destinations](lnd/202603181015-blinded-paths-privacy.md)
- Invoice reader (reply_path check): [BOLT 12 Invoice Reader Requirements](bolts/202603251340-bolt-12-invoice-reader-requirements.md)
- Flow context: [BOLT 12 Offer-to-Payment Flow Between Two LND Nodes](202603261030-bolt-12-offer-to-payment-flow.md)

## Backlinks
- [Feature Backlog](202603251500-Feature-Backlog.md)
- [Onion Message Pathfinding](202603261220-onion-message-pathfinding.md)
- [Bolt12 Mvp Direct Peers](202603261230-bolt12-mvp-direct-peers.md)
- [Bolt12 Implementation Strategy](202603261300-bolt12-implementation-strategy.md)
