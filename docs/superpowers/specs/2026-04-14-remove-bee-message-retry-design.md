# Design: Remove Bee Message Retry Mechanism

**Date:** 2026-04-14
**Status:** Approved

## Motivation

The current bee message retry mechanism silently retries failed messages up to 3 times before notifying the user. This behavior is undesirable because:

- Bee failures are typically logical errors (AI errors, task execution errors), not transient infrastructure errors, so retrying rarely helps.
- Retries can cause duplicate side effects (e.g., bee already sent a partial reply before failing).
- Users are not notified immediately; they only learn of the failure after all 3 retries are exhausted.

The desired behavior: **any bee execution failure immediately marks the message as failed and notifies the user.**

## Scope

This change removes the bee message retry mechanism entirely. It does **not** affect:

- `bee_outbound_messages.retry_count` — outbound message send retries (separate concern, untouched)
- `ResetFeedingToReceived` — startup recovery for messages stuck in `feeding` (not a retry mechanism)
- Worker task retry behavior (scheduled tasks resetting to pending on cron cycle)

## Architecture

### Current failure path

```
bee execution fails
  → rollback()
    → RollbackWithRetry() [increments retry_count in DB]
      → if retry_count < 3: status = 'received' (re-queued for next poll)
      → if retry_count >= 3: status = 'failed' + notify user
```

### New failure path

```
bee execution fails
  → failMessages()
    → MarkFailed() [status = 'failed' immediately]
    → notify user
```

## Database Changes

Add **migration 31** to drop the `retry_count` column from `bee_platform_messages`:

```sql
ALTER TABLE bee_platform_messages DROP COLUMN retry_count
```

The column is no longer meaningful once retries are removed. Dropping it keeps the schema honest.

## Code Changes

### `internal/domain/bee/constants.go`

- Remove `MaxRetries = 3` constant and its comment.

### `internal/domain/bee/feeder.go`

- Rename `rollback()` to `failMessages()`.
- Remove all `RetryCount`/`MaxRetries` logic.
- Replace `f.msgStore.RollbackWithRetry(ctx, ids, MaxRetries)` with a direct `MarkFailed` call.
- Notify failure for **all** messages immediately (no exhaustion check needed).

### `internal/infra/store/message_store.go`

- Remove `RetryCount int` field from `ClaimedMessage` struct.
- Remove `retry_count` from the `ClaimBatch` SELECT query and `Scan` call.
- Remove `RollbackWithRetry()` method entirely.
- Add `MarkFailed(ctx, ids)` method (thin wrapper around `UpdateStatusBatch` with `'failed'`).

### `internal/infra/model/execution.go`

- Remove `RetryCount int` and `MaxRetries int` fields from `FailureInfo`.
- Update comment on `FailureInfo`.

### `internal/domain/task/failure_notifier.go`

- Remove the `if info.RetryCount >= 0` conditional branch.
- Simplify failure notification to always use the non-retry format.

### `internal/domain/task/dispatcher.go`

- Remove `RetryCount: -1` sentinel from both `FailureInfo` literals (field no longer exists).

### `internal/infra/store/db.go`

- Add migration version 31: `drop_retry_count_from_platform_messages`.

## Test Changes

### `internal/domain/bee/feeder_test.go`

- Delete `TestFeeder_ExhaustedRetries_NotifiesFailure`.
- Add `TestFeeder_ImmediateFailureNotification`: verifies that a single bee failure immediately marks the message `failed` and triggers the notifier once.

### `internal/infra/store/message_store_test.go`

- Delete `TestMessageStore_RollbackWithRetry_BelowLimit`.
- Delete `TestMessageStore_RollbackWithRetry_ExhaustsRetries`.
- Add `TestMessageStore_MarkFailed`: verifies that `MarkFailed` sets status to `'failed'` for the given IDs.

### `internal/domain/task/failure_notifier_test.go`

- Remove `RetryCount` and `MaxRetries` fields from all `FailureInfo` literals.
- Remove the test case that asserts retry-count text appears in the notification.
- Verify the simplified notification format.

## Error Handling

- If `MarkFailed` returns an error, log it and continue (same as current behavior for `RollbackWithRetry` errors).
- If `NotifyTaskFailure` returns an error, log it and continue (unchanged).

## Testing Strategy

All changes are deletions or simplifications of existing logic. The test suite verifies:

1. A failing bee run immediately sets message status to `'failed'` (no intermediate `'received'`).
2. The failure notifier is called exactly once per failed message.
3. `MarkFailed` correctly updates the DB status.
4. Notification content no longer includes retry count text.
