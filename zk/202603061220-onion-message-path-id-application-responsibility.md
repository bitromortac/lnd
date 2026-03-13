# Path ID Validation Is the Application's Responsibility

A `path_id` is a secret byte sequence that the builder of a blinded path may
embed in the final hop's encrypted recipient data. Its purpose is to let the
recipient recognise that an incoming message is a reply to a specific previously
sent reply path, and to associate application state with that exchange. Without
it, a recipient cannot distinguish a genuine reply from an unrelated message
that happened to arrive on the same channel. The reply path mechanism is
described in [Reply Path Enables Anonymous
Response](spec/202603040920-reply-path-enables-anonymous-response.md).

The protocol specification requires the recipient to validate this field. LND
decodes it during final-hop processing but deliberately does not enforce the
BOLT #4 validation rules. The raw encrypted recipient data is forwarded to
subscribers unchanged, and `path_id` verification is left to the application
layer. This is a design boundary: the protocol plumbing handles transport and
decryption; secret management and state association belong to the application
that created the blinded path in the first place.

Tags: #architecture #lnd #onion-messages #privacy #protocol

## References
- Receiver flow that surfaces this data: [Onion Message Receiver Flow](202603061030-onion-message-receiver-flow.md)
- Reply path mechanism: [Reply Path Enables Anonymous Response](spec/202603040920-reply-path-enables-anonymous-response.md)
- Protocol requirements: [Onion Message Requirements](spec/202603041010-onion-message-requirements.md)

## Backlinks
- [Onion Message Receiver Flow](zk/202603061030-onion-message-receiver-flow.md)
