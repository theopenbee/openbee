# Design: Fix Worker Concurrent Execution Bug

**Date:** 2026-03-18
**Status:** Approved

## Background

A worker (AI employee) can be dispatched from multiple sessions simultaneously. The `TaskDispatcher` serializes execution per `sessionKey|workerID`, so two different sessions with tasks for the same worker each get their own independent queue and execute concurrently. This causes data corruption because both processes share the same `worker.WorkDir`.

## Root Cause

`queueKey(sessionKey, workerID)` in `internal/task_dispatcher/dispatcher.go` scopes serialization to a (session, worker) pair rather than the worker alone. Two sessions dispatching to the same worker create two separate queues that execute in parallel.

## Fix

Change `queueKey` to use `workerID` only. All tasks for a given worker — regardless of which session they originate from — are serialized in a single queue.

### `queueKey()` (1-line change)

```go
// Before
func queueKey(sessionKey, workerID string) string {
    return sessionKey + "|" + workerID
}

// After
func queueKey(_, workerID string) string {
    return workerID
}
```

`queueKey` is the single call site for key construction. All usages in `handleInbound`, `executeAsync`, and `handleResult` route through this helper — no inline key strings exist.

### `clearQueues(sessionKey)` (logic change)

The original implementation deleted queue entries by `sessionKey|` prefix. With workerID-only keys, prefix matching no longer applies. Instead, iterate all worker queues and filter out pending tasks that belong to the given session.

```go
func (d *TaskDispatcher) clearQueues(sessionKey string) {
    for key, state := range d.queues {
        var remaining []DispatchTask
        for _, t := range state.pendingTasks {
            if t.SessionKey != sessionKey {
                remaining = append(remaining, t)
            }
        }
        state.pendingTasks = remaining
        // Remove the entry entirely if the queue is now idle and empty.
        if !state.executing && len(state.pendingTasks) == 0 {
            delete(d.queues, key)
        }
    }
    if err := d.sessionStore.ClearSessionContexts(d.ctx, sessionKey); err != nil {
        slog.Error("clear session contexts", "component", "taskdispatcher",
            "sessionKey", sessionKey, "error", err)
    }
}
```

**Locking:** `d.queues` is only accessed inside the `Run()` goroutine's select loop. `clearQueues` is called exclusively from `case sessionKey := <-d.clearCh:` within that loop — never directly from any other goroutine. No mutex is needed.

**In-flight task behavior:** If the currently-executing task belongs to the session being cleared, it is left to complete naturally — `clearQueues` never killed running processes (same as before). The DB session context for this session is cleared immediately by `ClearSessionContexts`. When the in-flight task completes, `waitForResult` calls `UpsertSessionContext` only on `ExecStatusCompleted`; a task allowed to finish will persist its session ID. This is acceptable: the session context was cleared as a reset signal, and the in-flight task completing is a race that existed before this fix.

**`DispatchTask.SessionKey` field:** Already exists on the struct; no new field needed.

**Queue map entry lifecycle:** `handleResult` receives `res.queueKey` (set to `workerID` at `executeAsync` call time) and either starts the next pending task or deletes the map entry when the queue empties. This path is unchanged in logic; only the key format changes.

## Cross-Session Context Isolation

Session context in the store is keyed by `(sessionKey, workerID)`. When two sessions' tasks for the same worker execute sequentially in the new single queue, each task independently calls `GetSessionContext(ctx, task.SessionKey, task.WorkerID)` with its own `sessionKey`. There is no cross-session context leakage.

## What Does Not Change

- **`TaskScheduler`**: continues to claim and deliver all due tasks each tick; no per-worker grouping needed. See "DB State After Fix" for the consequence on `running` count.
- **`Manager.ExecuteWorker`**: has no internal concurrency guard; it relies entirely on the caller for serialization. The Dispatcher now ensures only one call at a time per worker.
- **`ClearSession` DB path**: `ClearSessionContexts` is called as before.

## DB State After Fix

After the fix, multiple tasks for the same worker may be marked `running` in the DB (claimed by `ClaimDueTasks` in the same tick), while only one is actually executing. This is a known limitation:

- **Recovery:** `RecoverRunning` on restart resets all `running` → `pending`, including tasks that were claimed but not yet executing. These are re-dispatched from the beginning, which is the existing idempotency contract.
- **`WorkDir` safety on recovery:** Tasks re-dispatched after restart execute sequentially (one per worker queue), so `WorkDir` is never accessed concurrently — the same guarantee the fix provides for the normal path.
- **Observability:** The `running` count per worker in the DB may be inflated by queued-but-not-executing tasks. This is a pre-existing limitation of the "claim-all" dispatch model and is out of scope for this fix.
- Optimizing `ClaimDueTasks` to claim one task per worker per tick was considered and rejected: it adds SQL complexity and introduces per-tick dispatch latency proportional to queue depth.

## Tests

Update `internal/task_dispatcher/dispatcher_test.go`:

1. **Update existing assertions** that construct or match queue keys containing `sessionKey|workerID` to use `workerID` only.

2. **Unit test — `queueKey` ignores sessionKey:** Confirm `queueKey("s1", "w1") == queueKey("s2", "w1")` and equals `"w1"`.

3. **Regression test — cross-session serial execution:** Use a mock `ExecuteWorker` that blocks on a channel until signaled. Dispatch two tasks for the same `workerID` from sessions S1 and S2. Assert that the mock is called exactly once initially. Signal the first call to complete. Assert the mock is then called a second time. This verifies FIFO sequential dispatch across sessions.

4. **Test — `clearQueues` filters by session:** Pre-populate a worker queue with one executing task from S1 and two pending tasks (one from S1, one from S2). Call `clearQueues("s1")`. Assert only S2's pending task remains. Assert the queue entry still exists (because `state.executing` is true). Assert `ClearSessionContexts` was called with `"s1"`.

5. **Test — empty queue entry cleaned up:** After `clearQueues` removes all pending tasks from an idle queue (`executing == false`), assert the map entry is deleted.
