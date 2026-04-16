# Pre-flight Session Context Write

**Date:** 2026-04-16  
**Status:** Approved

## Problem

`bee_session_contexts` records are only written **after** execution completes. This creates two issues:

1. **Visibility gap**: While bee or a worker is executing for the first time, `list_session_contexts` and `clear_session` cannot see the session. The session is invisible to the rest of the system until the first execution finishes.
2. **Recovery gap**: If the process crashes mid-execution before writing, there is no session record to resume from on the next attempt.

## Goal

Write to `bee_session_contexts` **before** execution starts so the session is immediately visible and recoverable.

## Design

### Overview

Introduce a **pre-flight upsert** in both the bee feeder and the worker dispatcher paths. The session ID is determined before execution starts (generated or fetched from the store), written to `bee_session_contexts`, and then passed into the execution runtime. The existing post-execution upsert is retained in both paths.

### Changes

#### 1. `ExecuteWorker` interface and `manager.go` — explicit `resume` flag

Currently `manager.go` derives `resume` from `sessionID != ""` and generates a UUID when `sessionID` is empty:

```go
resume := sessionID != ""
if sessionID == "" {
    sessionID = uuid.New().String()
}
```

This coupling breaks when the caller wants to pass a pre-assigned UUID for a brand-new session.

**Change:** Add an explicit `resume bool` parameter. Remove UUID generation from `manager.go` (the caller is now responsible):

```go
// Before
ExecuteWorker(ctx, workerID, input, sessionID string) (WorkerExecution, error)

// After
ExecuteWorker(ctx, workerID, input, sessionID string, resume bool) (WorkerExecution, error)
```

`manager.go` uses the `resume` parameter directly, no longer infers it from sessionID.

Affected files:
- `internal/domain/worker/manager.go` — signature + remove UUID generation + use explicit resume
- `internal/domain/task/dispatcher.go` — `ExecutionManager` interface + all `ExecuteWorker` call sites
- `internal/domain/task/dispatcher_test.go` — mock and test call sites

#### 2. `dispatcher.go` — pre-generate UUID, pre-flight upsert

`executeWithHint` (fresh session path) now generates the UUID and writes to the store before calling `ExecuteWorker`:

```go
func (d *TaskDispatcher) executeWithHint(ctx context.Context, task DispatchTask, instruction string) (model.WorkerExecution, error) {
    hint, err := d.workerSkillHint(task.WorkerID)
    if err != nil {
        return model.WorkerExecution{}, err
    }
    sessionID := uuid.New().String()
    if task.SessionKey != "" && task.WorkerID != "" {
        if err := d.sessionStore.UpsertSessionContext(ctx, task.SessionKey, task.WorkerID, sessionID, d.engineName); err != nil {
            log.Error("pre-flight upsert session context", zap.Error(err))
            // non-fatal: execution continues
        }
    }
    return d.manager.ExecuteWorker(ctx, task.WorkerID, hint+"\n"+instruction, sessionID, false)
}
```

`resolveExecution` (resume path) already has the session ID from the store. It performs the same pre-flight upsert before calling `ExecuteWorker`:

```go
// resume path in resolveExecution
if task.SessionKey != "" && task.WorkerID != "" {
    if err := d.sessionStore.UpsertSessionContext(ctx, task.SessionKey, task.WorkerID, sessionID, d.engineName); err != nil {
        log.Error("pre-flight upsert session context (resume)", zap.Error(err))
    }
}
exec, err := d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, sessionID, true)
```

The existing post-execution upserts in `waitForResult` (both completed and failed branches) are **retained unchanged**.

#### 3. `feeder.go` — pre-flight upsert for bee

After the session ID is determined (fetched or newly generated) and before `runner.Run()`:

```go
// After sessionID is resolved, before runner.Run()
if err := f.sessionStore.UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID, f.cfg.EffectiveEngine()); err != nil {
    log.Error("pre-flight upsert bee session context", zap.Error(err))
    // non-fatal: execution continues
}
```

The existing post-execution upsert in `processBeeGroup` (with its concurrent-clear guard) is **retained unchanged**.

### Data flow after the change

```
Bee path
─────────────────────────────────────────────────────────────────
sessionID resolved
    ↓
★ UpsertSessionContext (pre-flight)         ← NEW
    ↓
runner.Run() [blocks until complete]
    ↓
concurrent-clear check
    ↓
UpsertSessionContext (post-execution)       ← existing, retained


Worker path
─────────────────────────────────────────────────────────────────
sessionID resolved (fetched or uuid.New())
    ↓
★ UpsertSessionContext (pre-flight)         ← NEW
    ↓
ExecuteWorker(sessionID, resume) [non-blocking, returns exec]
    ↓
waitForResult [polls until complete/failed]
    ↓
UpsertSessionContext (post-execution)       ← existing, retained
```

### Error handling

Pre-flight upsert failures are **non-fatal**: logged, then execution proceeds. The session will still be written post-execution on success. This avoids a situation where a transient DB error prevents all executions.

### Edge cases

| Scenario | Behavior |
|----------|----------|
| Session cleared between pre-flight write and execution start | Clear wins. Post-execution write checks for concurrent clear (bee) or overwrites cleanly (worker). |
| `ExecuteWorker` fails to launch | Pre-flight record exists with a sessionID that was never used. Next execution for same sessionKey/workerID will overwrite via upsert. No orphan accumulation. |
| Resume attempt fails, falls back to fresh | `resolveExecution` calls `DeleteSessionContextForEngine` then `executeWithHint`. `executeWithHint` generates a new UUID and pre-writes it, correctly replacing the stale entry. |
| Bee crash mid-execution | Pre-flight record exists. On next attempt, `GetSessionContextForEngine` finds it, sets `resume=true`, and the AI engine attempts to resume. |

### What is NOT changed

- Schema of `bee_session_contexts` — no new columns
- Post-execution upsert logic in `waitForResult` and `processBeeGroup`
- Concurrent-clear guard in `processBeeGroup`
- All other callers of `ExecuteWorker` outside the dispatcher

## Testing

- `dispatcher_test.go`: update `mockExecManager` to accept `resume bool`; add tests that pre-flight upsert is called before `ExecuteWorker` for both fresh and resume paths
- `feeder_test.go`: add test that session context is written before `runner.Run()` is invoked
- Existing session-context tests in `session_store_test.go` require no changes
