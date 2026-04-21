# Stale Message Handling Design

**Date:** 2026-04-21
**Status:** Approved

## Problem

Due to network instability, messages can arrive at OpenBee out of order:

```
10:01  Message A sent — not received by OpenBee (network drop)
10:02  Message B sent — received and fully processed by OpenBee
10:10  Message A finally arrives (delayed delivery)
```

When Message A arrives at 10:10, processing it would be wrong: the user has already moved on
(evidenced by Message B). OpenBee has no mechanism to detect or discard such delayed messages.

## Decision

**Approach:** Extend `ClaimBatch` to mark out-of-order messages as `stale` before claiming.
**Notification:** Silent discard — no message sent to the user.

## Status Lifecycle

```
received  →  feeding  →  bee_processed
received  →  merged          (existing)
received  →  failed          (existing)
received  →  stale           (new)
```

A message is `stale` when its session has at least one `bee_processed` message with a
higher `received_at` (i.e., a newer message in the same conversation was already handled).

## Implementation

### Single-file change: `internal/infra/store/message_store.go`

**1. Add status constant**

```go
const (
    MsgStatusReceived     = "received"
    MsgStatusFeeding      = "feeding"
    MsgStatusMerged       = "merged"
    MsgStatusBeeProcessed = "bee_processed"
    MsgStatusFailed       = "failed"
    MsgStatusStale        = "stale"    // new
)
```

**2. Extend `ClaimBatch` — mark stale before claiming**

At the top of the `ClaimBatch` transaction, before the SELECT, execute:

```sql
UPDATE bee_platform_messages
SET    status = 'stale', updated_at = ?
WHERE  status = 'received'
  AND  EXISTS (
         SELECT 1
         FROM   bee_platform_messages b2
         WHERE  b2.session_key  = bee_platform_messages.session_key
           AND  b2.status       = 'bee_processed'
           AND  b2.received_at  > bee_platform_messages.received_at
       )
```

The rest of `ClaimBatch` (SELECT + UPDATE to `feeding`) is unchanged.

### Performance

- **Zero extra DB round-trips:** both the stale UPDATE and the existing claim SELECT/UPDATE
  run inside the same transaction that `ClaimBatch` already opens.
- **Index coverage:** the EXISTS subquery walks `idx_platform_messages_session`
  `(session_key, received_at DESC)` which already exists.
- **Execution frequency:** only on the Feeder ticker (default 2 s), not on the message
  ingestion hot path.

## Out of Scope

- No schema migration required (status is a TEXT column; SQLite accepts new values).
- No changes to Gateway, platform receivers, or Feeder orchestration logic.
- No user-facing notification when a message is discarded as stale.
