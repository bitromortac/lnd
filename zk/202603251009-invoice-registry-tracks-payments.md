# Invoice Registry Tracks Incoming Payments

The `invoices` subsystem manages the lifecycle of incoming payment requests.
When a user wants to receive funds over the Lightning Network, they create an
invoice containing a unique cryptographic hash and a required payment amount.
This invoice is stored in the local registry.

When a payment arrives at the [HTLC
Switch](202603181002-htlc-switch-routing.md), it queries the invoice registry to
check if the incoming hash corresponds to an expected payment. The registry
verifies that the payment conditions—such as the total amount or correct expiry
times—are met. If the invoice is a standard invoice, the registry supplies the
secret preimage to immediately settle the payment. If it is a "hold invoice,"
the registry instructs the switch to lock the funds but delays settlement until
the user explicitly reveals the preimage. This strict validation ensures that
incoming funds are only accepted if they match a pre-authorized request.

Tags: #architecture #payment

## References 
- Interacts with: [HTLC Switch](202603181002-htlc-switch-routing.md)
- Interacts with: [Channel Database](202603181004-channel-state-database.md)

## Backlinks
- [Lnd Architecture](202603181000-Lnd-Architecture.md)
