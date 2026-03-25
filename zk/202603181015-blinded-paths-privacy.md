# Blinded Paths Obscure Payment Destinations

A central privacy challenge in the standard Lightning Network is that the sender
must inherently know the exact topological location of the receiver to construct
the onion route. The `routing` subsystem introduces "Blinded Paths" (Route
Blinding) to mitigate this issue. This feature obscures the receiver's identity
from intermediate forwarding nodes.

The receiver provides an introduction node and an encrypted blob that only the
subsequent hops can unroll. When the [Pathfinding Router](202603181010-Pathfinding-Router.md)
constructs the path, it incorporates these blinded segments, potentially padding
the path with dummy hops or accumulated fee policies to hide the true distance
to the destination. The sender finds a route to the introduction node and
attaches the blinded segments to the [HTLC
Switch](202603181002-htlc-switch-routing.md), allowing the payment to reach the
recipient without the sender or any intermediate node being able to map the full
path to the final destination.

Tags: #routing #privacy #blinded-paths

## References
- Constructed by: [Pathfinding Router](202603181010-Pathfinding-Router.md)
- Processed by: [HTLC Switch](202603181002-htlc-switch-routing.md)

## Backlinks
- [Pathfinding Router](202603181010-Pathfinding-Router.md)
