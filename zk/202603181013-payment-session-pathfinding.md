# Payment Session Isolates Pathfinding Context

The `paymentSession` structure encapsulates the necessary state and constraints
required to compute optimal routes over the [Channel Database](202603181004-channel-state-database.md)
graph during a single payment lifecycle. It freezes critical pathfinding
parameters, such as maximum fee limits, total CLTV deltas, and bandwidth hints,
isolating the active payment from global graph mutations.

When the [Payment Lifecycle](202603181011-payment-lifecycle-state-machine.md)
requires a new route, the active session calculates the shortest path by
incorporating the dynamic edge probabilities supplied by [Mission
Control](202603181012-mission-control-probability.md). It uses a Dijkstra-based
heap to search the network, applying penalties (virtual costs) failed attempts
or low-probability edges. This ensures that the algorithm efficiently converges
on routes that are not merely the cheapest on paper, but the most statistically
likely to succeed without modifying global state.

Tags: #routing #pathfinding #graph-theory

## References
- Invoked by: [Payment Lifecycle](202603181011-payment-lifecycle-state-machine.md)
- Depends on: [Mission Control](202603181012-mission-control-probability.md)

## Backlinks
- [Pathfinding Router](202603181010-Pathfinding-Router.md)
- [Payment Lifecycle State Machine](202603181011-payment-lifecycle-state-machine.md)
- [Mission Control Probability](202603181012-mission-control-probability.md)
