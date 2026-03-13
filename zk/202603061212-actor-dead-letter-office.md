# Dead Letter Office: Observable Message Loss

The Dead Letter Office (DLO) is itself an actor. When a message cannot reach its
intended recipient — because the target actor has stopped, or the caller's
context was cancelled before the mailbox accepted the message — the sending
reference routes the message here instead of silently discarding it. The DLO
logs every arrival, providing an observable record of message loss.

The DLO's own DLO reference is nil. This breaks the potential routing loop: a
message that fails to reach the DLO is simply dropped rather than looping back.

The design keeps error-handling obligations off the caller. A component using
`Tell` does not need to check whether delivery succeeded — if it failed, the DLO
captures it. This is consistent with the fire-and-forget contract of [Tell:
Fire-and-Forget Actor Interaction](202603061205-actor-tell-fire-and-forget.md),
where the caller has no expectation of a result to begin with.

Tags: #architecture #actor #concurrency #lnd

## References
- System that owns it: [ActorSystem: Actor Lifecycle Management](202603061210-actor-system-lifecycle.md)
- Tell pattern (primary DLO consumer):
  [Tell: Fire-and-Forget Actor
  Interaction](202603061205-actor-tell-fire-and-forget.md)

## Backlinks
- [Actor Ask Future Promise](zk/202603061206-actor-ask-future-promise.md)
- [Actor System Lifecycle](zk/202603061210-actor-system-lifecycle.md)
- [Actor Pattern LND](zk/202603061215-Actor-Pattern-LND.md)
