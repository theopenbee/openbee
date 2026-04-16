# Design: /stop Cancels Unprocessed bee_platform_messages

**Date:** 2026-04-15
**Branch:** feature/stop-command

## Problem

When a user issues `/stop`, the current handler (`CommandInterceptor.handleStop`) stops active bee executions, cancels pending worker tasks, and clears the dispatcher queue. However, any messages in `bee_platform_messages` with `status = 'received'` for that session remain in the queue. On the next Feeder tick, these messages are claimed and processed — violating the semantics of `/stop` and allowing accumulated messages to enter the processing pipeline.

## Goals

- Cancel all `received` messages for the session when `/stop` is issued.
- Also cancel their associated `merged` messages (debounce-merged sub-messages).
- Introduce a distinct `cancelled` status (rather than reusing `failed`) for semantic clarity.
- Keep the change minimal and consistent with existing patterns.

## Non-Goals

- Cancelling `feeding` messages (already handled by stopping the bee execution process).
- Schema migration (no CHECK constraint on `bee_platform_messages.status`; `cancelled` can be used immediately).

## Design

### 1. MessageStore Layer

**New constant** in `internal/infra/store/message_store.go`:

```go
MsgStatusCancelled = "cancelled"
```

**New method:**

```go
// CancelReceivedBySessionKey cancels all 'received' messages for the given
// session and their associated 'merged' sub-messages.
// Returns the number of 'received' rows cancelled (not counting merged rows).
func (s *MessageStore) CancelReceivedBySessionKey(ctx context.Context, sessionKey string) (int64, error)
```

Implementation uses a transaction with two UPDATE statements:

1. Update all `received` rows for the session to `cancelled`, capture affected IDs.
2. Update all `merged` rows whose `merged_into` is in the captured ID set to `cancelled`.

Returns the count of `received` rows cancelled (used by the caller to determine whether anything was stopped).

**Impact on existing queries:**

| Query | Impact |
|---|---|
| `ClaimBatch` | Only claims `received`; `cancelled` rows are never claimed. No change needed. |
| `CountReceived` | Only counts `received`. No change needed. |
| `ListFiltered` | Filters by status if provided; `cancelled` is queryable as-is. No change needed. |
| `ResetFeedingToReceived` | Only touches `feeding`. No change needed. |

### 2. CommandInterceptor Layer

**New interface** in `internal/domain/bee/command_interceptor.go`:

```go
// messageCanceller cancels unprocessed platform messages for a session.
type messageCanceller interface {
    CancelReceivedBySessionKey(ctx context.Context, sessionKey string) (int64, error)
}
```

**Updated struct and constructor:**

```go
type CommandInterceptor struct {
    // ... existing fields ...
    msgCanceller messageCanceller  // new
}

func NewCommandInterceptor(
    ss *store.SessionStore,
    es *store.ExecutionStore,
    ts *store.TaskStore,
    stopper executionStopper,
    clearer sessionClearer,
    canceller messageCanceller,   // new parameter
    senders map[string]platform.PlatformSenderAdapter,
    engine string,
) *CommandInterceptor
```

**Updated `handleStop` sequence:**

1. Stop active bee executions (existing)
2. Cancel pending worker tasks (existing)
3. **Cancel unprocessed platform messages (new)** — call `msgCanceller.CancelReceivedBySessionKey`; if `n > 0`, set `stopped = true`
4. Clear dispatcher queues (existing)
5. Reply to user (existing)

Cancelling `received` messages contributes to `stopped = true`, so the reply says "stopped" rather than "nothing was running".

### 3. Wire-up

In the server initialization (wherever `NewCommandInterceptor` is called), pass `*store.MessageStore` as the `messageCanceller` argument. `MessageStore` satisfies the interface directly — no adapter needed.

### 4. Tests

**`internal/infra/store/message_store_test.go`** — add `TestCancelReceivedBySessionKey`:
- `received` messages for the session are marked `cancelled`
- Associated `merged` messages are marked `cancelled`
- Messages from other sessions are not affected
- Messages with `feeding`, `bee_processed`, `failed` status are not affected
- Returns correct count of cancelled `received` rows

**`internal/domain/bee/command_interceptor_test.go`** — verify:
- `handleStop` calls `msgCanceller.CancelReceivedBySessionKey` with the correct session key
- Cancelling messages contributes to `stopped = true` in the reply logic

## File Changelist

| File | Change |
|---|---|
| `internal/infra/store/message_store.go` | Add `MsgStatusCancelled` constant and `CancelReceivedBySessionKey` method |
| `internal/domain/bee/command_interceptor.go` | Add `messageCanceller` interface, update struct/constructor/`handleStop` |
| `internal/app/app.go` | Pass `s.messageStore` as `messageCanceller` to `NewCommandInterceptor` (line ~121) |
| `internal/infra/store/message_store_test.go` | Add `TestCancelReceivedBySessionKey` |
| `internal/domain/bee/command_interceptor_test.go` | Add/update stop command tests |
