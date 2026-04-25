# Design: Split `domain/worker/manager.go` by Responsibility

**Date:** 2026-04-24  
**Status:** Approved

## Background

`internal/domain/worker/manager.go` (374 lines) currently bundles three distinct responsibility clusters into a single file:

1. **Engine management** — resolving, listing, and validating AI engines
2. **Worker entity CRUD** — creating, updating, deleting workers; input params types; name validation
3. **Execution lifecycle** — spawning worker processes, monitoring output, stopping/cancelling executions

This makes the file harder to navigate and violates the single-responsibility principle at the file level.

## Goal

Reorganize the file into three focused files within the same `worker` package, matching the existing patterns in `domain/bee/` and `domain/task/`. No public API changes, no new types, no package boundary changes.

## Approach

File-level split within the same `internal/domain/worker` package. All methods remain on `*Manager`. The `Manager` struct definition stays in `manager.go` as the single source of truth for all fields.

This mirrors:
- `domain/bee/feeder.go` + `bee_process.go` (coordinator + process lifecycle)
- `domain/task/dispatcher.go` + `scheduler.go` + `task.go` (dispatch + scheduling + entity)

## Target File Structure

```
internal/domain/worker/
├── errors.go           — unchanged
├── names.go            — unchanged
├── names_test.go       — unchanged
├── manager.go          — Manager struct + NewManager + engine methods (~90 lines)
├── worker.go           — Worker CRUD + params types + name validation (~130 lines)
├── execution.go        — Execution lifecycle + process tracking (~140 lines)
└── manager_test.go     — unchanged (tests cover Manager behavior, not individual files)
```

## File Contents

### `manager.go` (coordinator + engine)

Retains the `Manager` struct definition (all fields, including `activeProcesses` and `mu`) and `NewManager`. Also owns the three engine-related methods that are shared by both CRUD and execution paths:

- `var log`
- `type Manager struct { ... }` — all fields unchanged
- `func NewManager(...) *Manager`
- `func (m *Manager) resolveEngine(w model.Worker) (string, ai.EngineAdapter)`
- `func (m *Manager) EnabledEngines() []string`
- `func (m *Manager) ValidateEngine(name string) error`

**Why engine methods here:** `resolveEngine` is called by both `CreateWorker` (in `worker.go`) and `ExecuteWorker` (in `execution.go`). Keeping it in `manager.go` avoids circular dependency and makes it a shared utility of the coordinator.

### `worker.go` (entity CRUD)

All worker entity operations and their supporting types:

- `type CreateWorkerParams struct { ... }`
- `type UpdateWorkerParams struct { ... }`
- `func (p UpdateWorkerParams) HasChanges() bool`
- `func (p UpdateWorkerParams) Validate(m *Manager) error`
- `func (p UpdateWorkerParams) ApplyTo(w *model.Worker)`
- `func (m *Manager) validateWorkerName(name, excludeID string) error`
- `func (m *Manager) CreateWorker(p CreateWorkerParams) (model.Worker, error)`
- `func (m *Manager) UpdateWorker(id string, p UpdateWorkerParams) (model.Worker, error)`
- `func (m *Manager) DeleteWorker(id string, deleteWorkDir bool) error`

### `execution.go` (runtime lifecycle)

All execution process management:

- `func (m *Manager) ExecuteWorker(ctx context.Context, workerID, triggerInput, sessionID string, resume bool) (model.WorkerExecution, error)`
- `func (m *Manager) launchRuntime(exec model.WorkerExecution, worker model.Worker, engine ai.EngineAdapter, timeout time.Duration, prompt string, resume bool) error`
- `func (m *Manager) monitorExecution(exec model.WorkerExecution, worker model.Worker, runRes ai.RunResult, cancel context.CancelFunc, logPath string)`
- `func (m *Manager) StopExecution(executionID string) error`
- `func (m *Manager) CancelExecution(_ context.Context, executionID string) error`

## Data Flow

```
ExecuteWorker (execution.go)
  └─ resolveEngine (manager.go)   ← shared by CRUD and execution
  └─ launchRuntime (execution.go)
       └─ monitorExecution (execution.go)  [goroutine]

CreateWorker (worker.go)
  └─ validateWorkerName (worker.go)
  └─ resolveEngine (manager.go)
  └─ engine.Prepare(...)
```

## Constraints

- All public method signatures remain identical — zero impact on callers
- No new exported types or interfaces introduced
- `Manager` struct fields are not moved or renamed
- `manager_test.go` is not modified (tests exercise `Manager` as a whole)
- Import lists in each new file will be trimmed to only what that file uses

## Out of Scope

- Extracting `ExecutionRunner` as a separate type (larger refactor, deferred)
- Moving engine management to `domain/enginecfg` (separate concern, deferred)
- Sub-package extraction (e.g., `worker/execution`) — not consistent with existing project style
- Splitting `manager_test.go` per file — low value, deferred
