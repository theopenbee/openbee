# Worker Engine Session Context Fix

## Problem

When a worker is configured with a custom engine (e.g., `pi`), the `bee_session_contexts` records
created during task execution use the system-default engine name (e.g., `kimi`) instead of the
worker's configured engine. This happens because `TaskDispatcher` holds a single global `engineName`
set at startup from `cfg.Bee.EffectiveEngine()`, and all session context operations use that global
value rather than the per-worker engine.

### Root Cause

`TaskDispatcher.engineName` is the system-wide default engine. It is passed to all session context
operations:

- `GetSessionContextForEngine(ctx, sessionKey, workerID, d.engineName)`
- `UpsertSessionContext(ctx, sessionKey, workerID, sessionID, d.engineName)`
- `DeleteSessionContextForEngine(ctx, sessionKey, workerID, d.engineName)`

Meanwhile, `worker.Manager.resolveEngine(w)` correctly selects the worker's configured engine for
execution, but the dispatcher is unaware of this and continues to key session contexts by the system
default.

### Impact

The `bee_session_contexts` table uses `(session_key, agent_id, engine)` as the composite primary key.
Storing contexts under the wrong engine key means:

1. **Session resumption uses a mismatched session ID.** A session ID created by the kimi engine is
   passed to the pi engine on resume, causing failures or undefined behavior.
2. **Session isolation is broken.** Workers with different engines may share the same session context
   slot.

## Decision

**Session context cleanup on engine change: not required.**

When a worker's engine is changed, old session context records under the previous engine key are
silently abandoned. On the first execution after the change, no record is found under the new engine
key, so a fresh session is created. Old records are harmless orphans that do not affect correctness.

This also handles existing dirty data from the bug: after the fix is deployed, workers with
`engine="pi"` but stale records stored under `engine="kimi"` will automatically start fresh sessions
under `"pi"` on next execution.

The only edge case — switching a worker's engine back to its original value — causes a stale resume
attempt. The existing fallback in `resolveExecution` handles this: resume failure automatically
retries as a fresh session.

## Solution

### Approach

Add a `resolveWorkerEngine(workerID string) string` helper to `TaskDispatcher`. It uses the existing
`WorkerLookup` interface to retrieve the worker's configured engine and falls back to
`d.engineName` when the lookup is unavailable or the worker has no engine set.

Replace all uses of `d.engineName` in session context operations with
`d.resolveWorkerEngine(task.WorkerID)`.

### Affected File

`internal/domain/task/dispatcher.go` — no other files require changes.

### New Helper

```go
// resolveWorkerEngine returns the engine name to use for session context operations.
// If workerLookup is set and the worker has a configured engine, that name is returned.
// Otherwise falls back to d.engineName (the system-default engine).
func (d *TaskDispatcher) resolveWorkerEngine(workerID string) string {
    if d.workerLookup != nil {
        if w, err := d.workerLookup.GetByID(workerID); err == nil && w.Engine != "" {
            return w.Engine
        }
    }
    return d.engineName
}
```

### Changes to Existing Methods

**`resolveExecution`** — resolve engine once, use it for both Get and Delete:

```go
engineName := d.resolveWorkerEngine(task.WorkerID)
sessionID, err := d.sessionStore.GetSessionContextForEngine(ctx, task.SessionKey, task.WorkerID, engineName)
// ...
d.sessionStore.DeleteSessionContextForEngine(ctx, task.SessionKey, task.WorkerID, engineName)
```

**`upsertSessionContext`** — replace `d.engineName` with `resolveWorkerEngine`:

```go
engineName := d.resolveWorkerEngine(task.WorkerID)
d.sessionStore.UpsertSessionContext(ctx, task.SessionKey, task.WorkerID, sessionID, engineName)
```

### DB Call Count

`workerLookup.GetByID` is a primary-key lookup. Per task execution, it is called:

| Path | Calls |
|------|-------|
| New session (`executeWithHint`) | 2 (1 in `workerSkillHint` + 1 in `upsertSessionContext`) |
| Resume session | 2 (1 in `resolveExecution` + 1 in `upsertSessionContext`) |
| `waitForResult` completion | 1–2 (in `upsertSessionContext`) |

All are indexed PK lookups. This is acceptable for the current workload.

> **Future optimization (out of scope):** Refactor `workerSkillHint` to return both the hint string
> and the resolved engine name, reducing the new-session path from 2 lookups to 1.

### Backward Compatibility

When `workerLookup` is nil or the worker has an empty `Engine` field, `resolveWorkerEngine` returns
`d.engineName`, preserving existing behavior exactly. All existing tests cover these paths and
require no modification.

## Testing

**New test:** `TestTaskDispatcher_WorkerEngine_UsedInSessionContext`

- Configure dispatcher with `WithEngine("kimi")` (system default) and `WithWorkerLookup` returning
  a worker with `Engine: "pi"`
- Run a task to completion
- Assert that the session context is stored under `engine="pi"`, not `"kimi"`

**Existing tests:** no changes required. Tests that do not set `WithWorkerLookup` exercise the
`workerLookup == nil` fallback path, which is unchanged.
