# Hold Invoice Expiration Threatens Channel Stability

Because [hold invoices](202603250834-hold-invoices.md) deliberately delay the
resolution of a payment, they inherently increase the risk of an underlying Hash
Time-Locked Contract (HTLC) timing out. When a hold invoice accepts a payment,
it locks the corresponding HTLCs on the incoming channel, freezing the funds and
tying up network liquidity.

Every incoming HTLC carries a CheckLockTimeVerify (CLTV) timeout. If an accepted
hold invoice remains unresolved as the blockchain height approaches this timeout,
the sender of the HTLC is forced to act to protect their funds. If the HTLC
timeout is reached and the receiver has neither settled nor failed the payment,
the sender must broadcast their commitment transaction to the blockchain, resulting
in a unilateral channel force-close.

To prevent this destructive event, nodes must actively monitor the expiration of
any unresolved hold invoices. The receiver's external logic must be designed to
either inject the preimage to settle the invoice or explicitly cancel the
invoice before the HTLC's timeout is breached, thereby failing the HTLC cleanly
off-chain and preserving the channel.

Tags: #invoices #htlc #dispute-resolution #lightning-network

## References
- Mitigates risk defined by: [Hold invoices](202603250834-hold-invoices.md)

## Backlinks
- [Invoices](202603250830-Invoices.md)
- [Hold Invoices](202603250834-hold-invoices.md)
