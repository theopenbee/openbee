# `/engine` Command Busy-Check Split Design

**Date:** 2026-04-29
**Status:** Approved

## Problem

The `/engine` command currently applies a single global `SystemBusyChecker` to both bee-level and worker-level engine switches. All three checks run regardless of the switch type:

| Check | Scope |
|-------|-------|
| `HasActiveMessages` | Any message in received/feeding |
| `HasActiveExecutions` | Any execution (bee + all workers) in pending/running |
| `HasActiveImmediateTasks` | Any immediate task (all workers) in pending/running |

This means `/engine codex alice` is rejected whenever any other worker is running — even if alice itself has no active work. The restriction is too coarse.

## Goal

Split busy-check conditions by scope so each switch type only blocks on its own relevant activity:

- **Bee-level** (`/engine codex`): block only on bee's own activity
- **Worker-level** (`/engine codex alice`): block only on that specific worker's activity

## Design

### 1. Store Layer — New Methods

**`ExecutionStore`** gains two scoped query methods:

```go
// HasActiveBeeExecutions reports whether bee-owned executions (worker_id IS NULL)
// with status pending or running exist.
func (s *ExecutionStore) HasActiveBeeExecutions(ctx context.Context) (bool, error)
// SQL: WHERE worker_id IS NULL AND status IN ('pending', 'running')

// HasActiveExecutionsByWorkerID reports whether the given worker has any
// pending or running executions.
func (s *ExecutionStore) HasActiveExecutionsByWorkerID(ctx context.Context, workerID string) (bool, error)
// SQL: WHERE worker_id = ? AND status IN ('pending', 'running')
```

The existing `HasActiveExecutions` (global) is kept unchanged — other callers may depend on it.

**`TaskStore`** gains one scoped query method:

```go
// HasActiveImmediateTasksByWorkerID reports whether the given worker has any
// pending or running immediate tasks.
func (s *TaskStore) HasActiveImmediateTasksByWorkerID(ctx context.Context, workerID string) (bool, error)
// SQL: WHERE worker_id = ? AND type = 'immediate' AND status IN ('pending', 'running')
```

### 2. Interface Layer — `command/engine.go`

`SystemBusyChecker` is removed from `EngineCommandHandler`. Two new focused interfaces replace it:

```go
// BeeBusyChecker gates bee-level engine switches.
type BeeBusyChecker interface {
    HasActiveMessages(ctx context.Context) (bool, error)      // reuses MessageActivityChecker
    HasActiveBeeExecutions(ctx context.Context) (bool, error) // new
}

// WorkerBusyChecker gates worker-level engine switches.
// All checks are scoped to a single worker by ID.
type WorkerBusyChecker interface {
    HasActiveExecutionsByWorkerID(ctx context.Context, workerID string) (bool, error)
    HasActiveImmediateTasksByWorkerID(ctx context.Context, workerID string) (bool, error)
}
```

Three new single-method interfaces are added (following the existing `MessageActivityChecker` / `ExecutionActivityChecker` pattern) and composed into the two checkers above:

```go
type BeeExecutionActivityChecker interface {
    HasActiveBeeExecutions(ctx context.Context) (bool, error)
}
type WorkerExecutionActivityChecker interface {
    HasActiveExecutionsByWorkerID(ctx context.Context, workerID string) (bool, error)
}
type WorkerTaskActivityChecker interface {
    HasActiveImmediateTasksByWorkerID(ctx context.Context, workerID string) (bool, error)
}

func NewBeeBusyChecker(msg MessageActivityChecker, exec BeeExecutionActivityChecker) BeeBusyChecker
func NewWorkerBusyChecker(exec WorkerExecutionActivityChecker, task WorkerTaskActivityChecker) WorkerBusyChecker
```

`ExecutionStore` and `TaskStore` satisfy the new single-method interfaces directly (no adapter needed).

### 3. Handler Logic — `EngineCommandHandler`

Replace the single `busy SystemBusyChecker` field with two scoped fields:

```go
type EngineCommandHandler struct {
    ...
    beeBusy    BeeBusyChecker
    workerBusy WorkerBusyChecker
    ...
}
```

`checkBusy` splits into two methods:

```go
func (h *EngineCommandHandler) checkBeeBusy(ctx context.Context) (string, bool)
func (h *EngineCommandHandler) checkWorkerBusy(ctx context.Context, workerID string) (string, bool)
```

**Worker-level command execution order changes** (worker lookup moves before busy check):

Current order:
```
1. validateEngine
2. checkBusy (global)
3. handleWorkerEngine → GetByName → UpdateEngine
```

New order for worker path:
```
1. validateEngine
2. GetByName(workerName)       ← moved earlier; return error immediately if not found
3. checkWorkerBusy(workerID)
4. UpdateEngine
```

Bee path is unchanged except `checkBusy` → `checkBeeBusy`.

### 4. Wiring — `app.go`

```go
// Before
busyChecker := command.NewSystemBusyChecker(s.msgStore, s.execStore, s.taskStore)
engineCmdHandler := command.NewEngineCommandHandler(..., busyChecker, ...)

// After
beeBusy    := command.NewBeeBusyChecker(s.msgStore, s.execStore)
workerBusy := command.NewWorkerBusyChecker(s.execStore, s.taskStore)
engineCmdHandler := command.NewEngineCommandHandler(..., beeBusy, workerBusy, ...)
```

## Blocking Conditions Summary

| Switch type | Blocked when |
|-------------|-------------|
| `/engine <engine>` (bee) | Active messages (received/feeding) **OR** active bee executions (worker_id IS NULL, pending/running) |
| `/engine <engine> <worker>` (worker) | Target worker has active executions (pending/running) **OR** active immediate tasks (pending/running) |

## Files Changed

| File | Change |
|------|--------|
| `internal/infra/store/execution_store.go` | Add `HasActiveBeeExecutions`, `HasActiveExecutionsByWorkerID` |
| `internal/infra/store/task_store.go` | Add `HasActiveImmediateTasksByWorkerID` |
| `internal/domain/command/engine.go` | Replace `SystemBusyChecker` with `BeeBusyChecker` + `WorkerBusyChecker`; refactor handler |
| `internal/domain/command/engine_test.go` | Update mocks; add scoped busy-check test cases |
| `internal/app/app.go` | Update wiring |

## Out of Scope

- i18n message text changes (existing `BusyMessages`, `BusyExecutions`, `BusyTasks` strings are reused)
- Changes to `HasActiveExecutions` or `HasActiveImmediateTasks` (kept for other consumers)
