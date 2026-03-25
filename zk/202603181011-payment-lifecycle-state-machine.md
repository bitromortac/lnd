# Payment Lifecycle Orchestrates Routing Attempts

The `paymentLifecycle` serves as the resilient state machine that guides an
outgoing payment through the Lightning Network. It does not simply select a
route and fire-and-forget; instead, it orchestrates an iterative retry loop to
handle the network's dynamic, eventually-consistent state.

During its execution loop, the lifecycle requests a route from the active
[Payment Session](202603181013-payment-session-pathfinding.md) and dispatches an
HTLC to the switch. Crucially, it must handle asynchronous failures, such as
when an intermediate node lacks liquidity or goes offline. The lifecycle catches
these errors, reports them back to [Mission
Control](202603181012-mission-control-probability.md), and uses this newly
discovered network state to compute a different path. This continuous
probe-and-correct mechanism ensures high payment success rates despite the
hidden nature of remote channel balances.

Tags: #routing #payment #algorithm

## References
- Invoked by: [Pathfinding Router](202603181010-Pathfinding-Router.md)
- Interacts with: [HTLC Switch](202603181002-htlc-switch-routing.md)

## Backlinks
- [Pathfinding Router](202603181010-Pathfinding-Router.md)
- [Mission Control Probability](202603181012-mission-control-probability.md)
- [Payment Session Pathfinding](202603181013-payment-session-pathfinding.md)
- [Multi Path Payment Sharding](202603181014-multi-path-payment-sharding.md)
