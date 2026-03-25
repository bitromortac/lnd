# Mission Control Models Channel Liquidity Probability

In the Lightning Network, the local capacity of a remote channel is
fundamentally hidden from the routing graph. The `MissionControl` subsystem
compensates for this lack of state by acting as a probabilistic tracking engine
that models the likelihood of a payment successfully traversing a given edge.

When a payment attempt fails due to insufficient liquidity, the [Payment
Lifecycle](202603181011-payment-lifecycle-state-machine.md) reports this failure
back to Mission Control. It uses this historical data to dynamically adjust its
internal probability estimators (such as the Apriori or Bimodal distributions).
This enables the [Pathfinding Router](202603181010-Pathfinding-Router.md) to
calculate the "virtual cost" of an attempt and trade off potentially cheaper,
shorter routes against routes that have a much higher mathematical probability
of succeeding based on recent probing.

Tags: #routing #mission-control #probability

## References
- Used by: [Payment Session](202603181013-payment-session-pathfinding.md)

## Backlinks
- [Pathfinding Router](202603181010-Pathfinding-Router.md)
- [Payment Lifecycle State Machine](202603181011-payment-lifecycle-state-machine.md)
- [Payment Session Pathfinding](202603181013-payment-session-pathfinding.md)
