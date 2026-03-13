# The Actor Model: Origins and Core Properties

The Actor Model is a mathematical theory of concurrent computation proposed by
Carl Hewitt, Peter Bishop, and Richard Steiger at MIT in 1973. Its central claim
is that all computation can be expressed through actors — entities that receive
messages, change their local state, create new actors, and send messages in
response. Gul Agha's 1985 PhD thesis formalised the semantics.

The first production-grade implementation was **Erlang/OTP** (Ericsson, 1987),
which proved the model in high-availability telecom systems. Its "let it crash"
philosophy and supervisor trees showed how fault isolation at the actor boundary
could be used structurally, not just defensively. **Akka** (2009) later brought
typed actors to the JVM. Go's goroutines-plus-channels share the spirit but are
not a formal actor system; LND constructs the actor abstraction on top of them.

## Properties

**Private state.** An actor's data is invisible to all other actors. The only
way to influence it is to send a message. This eliminates data races and the
need for mutexes inside actor logic.

**Sequential processing.** Each actor processes exactly one message at a time,
in the order received. Concurrency happens *between* actors — within a single
actor the code is effectively single-threaded. This makes actor-internal logic
straightforward to reason about and test.

**Asynchronous message passing.** The mailbox is a buffer between sender and
receiver. A sender deposits a message and returns immediately — it never blocks
waiting for the actor to process it. This decoupling is what enables high
throughput under load.

**Location transparency.** Callers hold a reference (in LND, an `ActorRef`)
rather than a pointer to the actor itself. The reference abstracts away where
the actor runs. In distributed systems this makes local and remote actors
uniform. In LND it means callers cannot reach into actor state directly, even
within a single process.

**Failure isolation.** Because actors share no memory, a panic or error inside
one actor cannot corrupt another's state. Combined with supervisor patterns, the
boundary makes crash recovery tractable.

Tags: #architecture #actor #concurrency #theory

## References
- LND implementation: [ActorSystem: Actor Lifecycle Management](202603061210-actor-system-lifecycle.md)
- Tell pattern: [Tell: Fire-and-Forget Actor Interaction](202603061205-actor-tell-fire-and-forget.md)
- Ask pattern: [Ask: Request-Response via Future and Promise](202603061206-actor-ask-future-promise.md)

## Backlinks
- [Actor Tell Fire And Forget](zk/202603061205-actor-tell-fire-and-forget.md)
- [Actor Ask Future Promise](zk/202603061206-actor-ask-future-promise.md)
- [Actor Sealed Message Capability](zk/202603061207-actor-sealed-message-capability.md)
- [Actor System Lifecycle](zk/202603061210-actor-system-lifecycle.md)
- [Actor Receptionist Service Discovery](zk/202603061211-actor-receptionist-service-discovery.md)
- [Actor Behavior Strategy](zk/202603061214-actor-behavior-strategy.md)
- [Actor Pattern LND](zk/202603061215-Actor-Pattern-LND.md)
