# Invoice Data Model Encapsulates Payment Constraints

Invoices act as the authoritative contract defining the conditions under which a
Lightning payment may be accepted. They encapsulate both the cryptographic
material needed to settle an incoming payment and the temporal constraints
ensuring the transaction is secure. An invoice requires a payment hash, which
serves as the primary identifier, and a preimage, which is the secret revealed
upon successful settlement.

Beyond the core cryptographic material, the invoice data model specifies the
exact value expected from the payer. It also defines a final CheckLockTimeVerify
(CLTV) delta, which represents the minimum number of blocks required before the
associated Hash Time-Locked Contract (HTLC) expires. This safety margin ensures
the receiving node has adequate time to claim the funds on-chain if a dispute
occurs.

Modern invoices augment this model with a payment address, a random 32-byte
value that prevents probing attacks and ensures the payer is satisfying the
intended invoice rather than reusing an old payment hash. The combination of
these parameters allows the [invoice registry](202603250838-invoice-registry.md)
to rigorously validate incoming HTLCs before transitioning the invoice's state.

Tags: #invoices #lightning-network #htlc #architecture

## References
- Defined in the domain collection: [Invoices](202603250830-Invoices.md)

## Backlinks
- [Invoices](202603250830-Invoices.md)
- [Invoice Creation Flow](202603250833-invoice-creation-flow.md)
