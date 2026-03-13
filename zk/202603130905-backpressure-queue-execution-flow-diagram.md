# Backpressure Queue Execution Flow

This flowchart illustrates the path of an incoming message into the
`BackpressureMailbox`. It highlights the
[Random Early Drop (RED)](202603061010-onion-message-backpressure-red.md) logic
and the possible error paths when attempting to enqueue an item.

The drop predicate allows the actor system to shed load proactively before the
underlying channel becomes fully saturated.

```mermaid
flowchart TD
    Start("Incoming Onion Message") --> ActorSend("Actor.Send(env)")
    ActorSend --> MailboxSend("BackpressureMailbox.Send / TrySend")

    MailboxSend --> IsClosed("Is Closed?")
    IsClosed -- Yes --> DropClosed("Return False / ErrQueueClosed")
    IsClosed -- No --> DropCheck("DropPredicate Should Drop?")

    DropCheck -- Yes --> REDDrop("Return False / ErrQueueFullAndDropped")
    DropCheck -- No --> ChanFullCheck("Channel Full?")

    ChanFullCheck -- Yes --> WaitOnChannel("Block on Channel/Ctx")
    WaitOnChannel -- Context Done --> ErrContext("Return Ctx.Err")
    WaitOnChannel -- Space Available --> SuccessEnqueue("Add to chan")

    ChanFullCheck -- No --> SuccessEnqueue
    SuccessEnqueue --> ReturnSuccess("Return Success / nil")
```

Tags: #diagram #architecture #lnd #concurrency #onion-messages #skip-lint

## References

## Backlinks
- [Onion Message Backpressure Red](zk/202603061010-onion-message-backpressure-red.md)
