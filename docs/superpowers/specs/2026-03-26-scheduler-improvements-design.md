# Scheduler Improvements Design

**Date:** 2026-03-26
**Status:** Draft
**Scope:** `internal/task_scheduler`, `internal/task_dispatcher`, `internal/store`

---

## Background

This document captures the design conclusions from a technical brainstorming session on the scheduler and dispatcher subsystems. Two improvement areas were identified:

1. **Scheduled task `next_run_at` update race** — the current 24h sentinel pattern is a known temporary workaround with a crash-recovery gap.
2. **Task cancellation** — there is no mechanism to interrupt an actively executing task or cleanly remove a task from the in-memory dispatcher queue by task ID.

---

## Problem 1: 24h Sentinel in `ClaimDueTasks`

### Current behavior

`ClaimDueTasks` (in `TaskStore`) handles scheduled tasks by setting `next_run_at = nowMS + 24h` inside the transaction as a sentinel — this prevents the task from being immediately re-claimed on the next poll. After the transaction commits, `Scheduler.poll()` calls `cron.ParseStandard` to compute the real next run time and calls `UpdateNextRunAt` to overwrite the sentinel.

This is a **two-step update**:

```
Transaction commits  →  [window]  →  UpdateNextRunAt (real value)
```

If the process crashes inside this window, the scheduled task silently sleeps for up to 24 hours before being picked up again.

### Root cause

The cron-parsing logic (`robfig/cron`) lives in `task_scheduler`, not in `store`. The store layer has no way to compute the real `next_run_at` at claim time.

### Proposed fix

Split claim into two steps: a read-only pre-scan to gather cron expressions, followed by a single transactional claim that writes the real `next_run_at` values atomically.

1. Add `PeekDueScheduledTasks(ctx, nowMS) ([]model.Task, error)` to `TaskStore` — a read-only query returning ID + CronExpr for scheduled tasks that are due (`next_run_at IS NULL OR next_run_at <= nowMS`). No locking, no update.
2. `Scheduler.poll()` calls `PeekDueScheduledTasks`, parses each cron expression, and builds a `map[taskID]int64` of computed `next_run_at` values.
3. Pass the map into `ClaimDueTasks` (signature updated to accept `scheduledNextRuns map[string]int64`). The transaction uses the pre-computed values directly — no sentinel.
4. `ClaimDueTasks` still atomically marks immediate/countdown tasks as `running` in the same transaction.

The pre-scan and the claim are separate DB calls, so there is a brief window where another process could observe the same due tasks. In practice openbee runs as a single process, so this is not a concern. The transaction still provides atomicity: if the process crashes after the pre-scan but before the commit, `next_run_at` is unchanged and the task will be claimed on the next poll.

This eliminates the crash-recovery gap entirely. The 24h sentinel code in `ClaimDueTasks` and the `UpdateNextRunAt` call in `poll()` are both removed.

---

## Problem 2: Task Cancellation

### Current gaps

| Scenario | Current behavior |
|---|---|
| Pending task in DB, not yet in memory | `CancelTask` / `CancelBySessionKey` (store) updates DB — sufficient |
| Pending task in Dispatcher's in-memory queue | `ClearSession` removes by session key only; no per-task-ID removal |
| Actively executing task (goroutine in `waitForResult`) | No mechanism to interrupt; goroutine runs up to 30-minute timeout |
| Underlying worker process | `ExecutionManager` has no cancel interface |

### Proposed solution: A+C combination

#### C — Add `CancelExecution` to `ExecutionManager`

```go
type ExecutionManager interface {
    ExecuteWorker(ctx context.Context, workerID, input, sessionID string) (model.WorkerExecution, error)
    CancelExecution(ctx context.Context, executionID string) error
}
```

The implementation kills the worker process. This is a prerequisite for truly stopping an in-flight execution.

#### A — Per-task cancel context in `TaskDispatcher`

Add to `TaskDispatcher`:

```go
cancelFuncs map[string]context.CancelFunc  // taskID → cancel func
cancelCh    chan string                     // receives taskID cancel requests
```

**Creating the cancel func (Run loop owns this):**

The cancel func is created in `handleInbound` (inside the Run loop), **before** launching the goroutine. The derived `taskCtx` is passed into `executeAsync`.

```go
func (d *TaskDispatcher) handleInbound(task DispatchTask) {
    key := queueKey(task.SessionKey, task.WorkerID)
    state := d.getOrCreateQueue(key)

    if !state.executing {
        state.executing = true
        taskCtx, cancel := context.WithCancel(d.ctx)
        if task.TaskID != "" {
            d.cancelFuncs[task.TaskID] = cancel
        }
        go d.executeAsync(taskCtx, key, task)
    } else {
        state.pendingTasks = append(state.pendingTasks, task)
    }
}
```

The same pattern applies in `handleResult` when the next pending task is picked up.

**Cleanup:** `handleResult` deletes the cancel func after a task completes:

```go
delete(d.cancelFuncs, res.task.TaskID)
```

**`handleCancel` (Option A — traverse queues):**

```go
func (d *TaskDispatcher) handleCancel(taskID string) {
    // Remove from pending queues (queues are small in practice)
    for key, state := range d.queues {
        state.pendingTasks = slices.DeleteFunc(state.pendingTasks,
            func(t DispatchTask) bool { return t.TaskID == taskID })
        if !state.executing && len(state.pendingTasks) == 0 {
            delete(d.queues, key)
        }
    }
    // Interrupt executing goroutine if present
    if cancel, ok := d.cancelFuncs[taskID]; ok {
        cancel()
        delete(d.cancelFuncs, taskID)
    }
}
```

**Public `CancelTask` method (best-effort, non-blocking):**

```go
func (d *TaskDispatcher) CancelTask(ctx context.Context, taskID string) error {
    // 1. Persist cancellation to DB
    if err := d.taskStore.CancelTask(ctx, taskID); err != nil {
        return err
    }
    // 2. Signal Run loop (best-effort, fire-and-forget)
    select {
    case d.cancelCh <- taskID:
    default:
        log.Warn("cancelCh full, in-memory cancel dropped", zap.String("taskID", taskID))
    }
    return nil
}
```

The caller receives a response immediately. The actual goroutine interruption and worker kill happen asynchronously.

**`executeAsync` — Method Y for the executionID timing race:**

Between `resolveExecution` returning and `waitForResult` starting, a cancel may have already fired. Method Y handles this explicitly:

```go
exec, err := d.resolveExecution(taskCtx, task, instruction)
if err != nil {
    // existing error handling
    return
}

// Method Y: if cancelled while resolveExecution was in-flight, kill now
if taskCtx.Err() != nil {
    d.manager.CancelExecution(context.Background(), exec.ID) //nolint:errcheck
    return
}

d.taskStore.SetExecution(taskCtx, task.TaskID, exec.ID, model.TaskStatusRunning) //nolint:errcheck
d.waitForResult(taskCtx, exec.ID, task)
```

`waitForResult` needs one addition: when it exits due to `ctx.Done()` (i.e. the task was cancelled), it must call `CancelExecution` so the worker process is killed. Without this, the goroutine exits cleanly but the worker keeps running.

```go
func (d *TaskDispatcher) waitForResult(ctx context.Context, executionID string, task DispatchTask) {
    // ... existing poll loop ...
    select {
    case <-time.After(pollInterval):
    case <-ctx.Done():
        // Task was cancelled — kill the worker process
        d.manager.CancelExecution(context.Background(), executionID) //nolint:errcheck
        return
    }
}
```

Method Y (the check after `resolveExecution`) and this `waitForResult` change together ensure `CancelExecution` is called regardless of when the cancel arrives.

---

## Edge cases

**Cancel arrives before goroutine starts:**
`handleInbound` already stored the cancel func in `cancelFuncs`. `handleCancel` finds it and calls cancel. By the time the goroutine runs, `taskCtx` is already cancelled, so `resolveExecution` fails fast and Method Y cleans up.

**Cancel arrives for a task not in memory (already completed or never dispatched):**
`handleCancel` iterates queues (nothing found), checks `cancelFuncs` (nothing found), and exits silently. The DB update in `CancelTask` is the authoritative record.

**`cancelCh` is full (burst of cancellations):**
The DB is updated synchronously before the channel send. The Scheduler will not re-claim the task from DB (status is `cancelled`). However, if the in-memory signal is dropped, a task already sitting in a Dispatcher pending queue will still be dispatched. In practice `cancelCh` has capacity 8 and bursts beyond that are unlikely; if tighter guarantees are needed the capacity can be raised. This is an accepted trade-off of the best-effort cancellation model.

---

## Summary of changes

| Component | Change |
|---|---|
| `ExecutionManager` interface | Add `CancelExecution(ctx, executionID) error` |
| `TaskStore` | Add `PeekDueScheduledTasks(ctx, nowMS)` read-only query |
| `TaskStore.ClaimDueTasks` | Remove 24h sentinel; accept pre-computed `scheduledNextRuns map[string]int64` |
| `Scheduler.poll` | Pre-scan cron tasks, compute `next_run_at`, pass into claim; remove `UpdateNextRunAt` call |
| `TaskDispatcher` | Add `cancelFuncs map`, `cancelCh`, `CancelTask` method, `handleCancel` |
| `TaskDispatcher.handleInbound` | Create per-task cancel context before launching goroutine |
| `TaskDispatcher.handleResult` | Delete cancel func on task completion; create new one for next pending task |
| `TaskDispatcher.executeAsync` | Accept `taskCtx`; apply Method Y after `resolveExecution` |
| `TaskDispatcher.waitForResult` | Call `CancelExecution` on `ctx.Done()` exit |
