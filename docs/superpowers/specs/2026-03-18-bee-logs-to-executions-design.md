# Design: Bee Process Logs → executions Table

**Date:** 2026-03-18
**Status:** Approved

## Background

Currently, each bee (manager Claude Code process) invocation writes its output to a file under `~/.openbee/bee-logs/<sessionID>_<timestamp>.log`. This is disconnected from the `executions` table that already tracks worker process runs. The goal is to store each bee invocation as a row in the `executions` table, making bee runs observable in the same place as worker runs.

## Requirements

- Each call to `processBeeGroup` produces exactly one row in `executions`.
- The row stores: session ID, trigger input (prompt), status lifecycle, logs (stdout+stderr accumulated in memory), PID, started_at, completed_at.
- Bee executions are distinguished from worker executions by a NULL `worker_id`.
- File-based logging (`bee-logs` directory) is removed entirely.
- No data migration needed — project is pre-release, existing rows are discarded.

## Schema Change

Add a new migration (version 16) that drops and recreates `executions` with `worker_id` nullable and no FK constraint. Since the project is pre-release, discarding existing execution rows is acceptable. Editing migration 2 in-place is insufficient because the migration runner tracks applied versions — any developer who already ran the app would not see the change.

Migration 16 drops the table (which also drops indexes 5 and 6 created by migrations 5 and 6), then recreates the table and both indexes:

```sql
-- Migration 16
DROP TABLE IF EXISTS executions;
CREATE TABLE executions (
    id             TEXT PRIMARY KEY,
    worker_id      TEXT,                  -- NULL for bee executions
    session_id     TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending',
    ai_process_pid INTEGER NOT NULL DEFAULT 0,
    trigger_input  TEXT NOT NULL DEFAULT '',
    result         TEXT NOT NULL DEFAULT '',
    logs           TEXT NOT NULL DEFAULT '',
    started_at     INTEGER,
    completed_at   INTEGER
);
CREATE INDEX idx_executions_worker_id ON executions(worker_id);
CREATE INDEX idx_executions_session_id ON executions(session_id);
```

Migration 2 (original CREATE TABLE) is left as-is so migration history stays consistent.

## model.WorkerExecution Change (`internal/model/execution.go`)

`WorkerID` changes from `string` to `*string` to correctly scan SQL NULL:

```go
type WorkerExecution struct {
    ID           string          `json:"id" db:"id"`
    WorkerID     *string         `json:"worker_id,omitempty" db:"worker_id"`  // nil for bee
    ...
}
```

`omitempty` on a `*string` omits the field when nil, avoiding `null` in API responses for bee executions.

## ExecutionStore Changes (`internal/store/execution_store.go`)

Three changes:

**1. `Create` method** — `WorkerID` struct literal must use pointer:
```go
exec := model.WorkerExecution{
    WorkerID: &workerID,   // was: WorkerID: workerID
    ...
}
```

**2. `scanExecution`** — `database/sql` correctly scans SQL NULL into `*string` (sets nil). No logic change needed, but the field type change must be consistent.

**3. Add `CreateBeeExecution`:**
```go
func (s *ExecutionStore) CreateBeeExecution(sessionID, triggerInput string) (model.WorkerExecution, error)
```
Inserts a row with `worker_id = NULL`. Returns the created `WorkerExecution` with `WorkerID == nil`.

## ExecutionStore Tests (`internal/store/execution_store_test.go`)

**Update existing test** `TestExecutionStore_CreateAndGet` — dereference pointer in comparison:
```go
if *got.WorkerID != w.ID {   // was: got.WorkerID != w.ID
```

**Add new test** for `CreateBeeExecution` covering:
- Inserts without error
- `GetByID` returns the row without scan error
- `WorkerID` is nil on the returned struct

## Feeder Changes (`internal/bee/feeder.go`)

### Struct & Constructor

Add `execStore *store.ExecutionStore` field to `Feeder`. Update `NewFeeder` signature accordingly.

### `processBeeGroup` Lifecycle

`CreateBeeExecution` is called only after both `GetSessionContext` and `runner.Run` succeed — i.e., immediately after confirming the process started. This avoids orphaned `pending` rows from early-return error paths.

Note: `runner.Run` currently discards the `*claude.Process` return value (`_`). Change this to a named variable `proc` so `proc.PID()` is accessible. In tests, `mockBeeRunner` returns `&claude.Process{}` with no cmd, so `proc.PID()` returns 0 — acceptable.

```
GetSessionContext → (fail: rollback, return)
runner.Run        → proc, outputCh (fail: rollback, return)
CreateBeeExecution → execID
UpdatePID(execID, proc.PID())
drainBeeOutput    → logs, err
  success: UpdateLogs(execID, logs)
           UpdateResult(execID, "", ExecStatusCompleted)
  failure: UpdateLogs(execID, logs)
           UpdateResult(execID, err.Error(), ExecStatusFailed)
           rollback(msgs)
           return
UpsertSessionContext
MarkBeeProcessed
```

### `drainBeeOutput` Signature Change

```go
// Before
func (f *Feeder) drainBeeOutput(ch <-chan claude.Output, sessionID string) error

// After
func (f *Feeder) drainBeeOutput(ch <-chan claude.Output) (string, error)
```

Accumulates stdout/stderr in a `strings.Builder`. On `OutputError`, appends the error line and returns partial logs plus an error. On `OutputDone`, returns full logs and nil. All file I/O (`os.MkdirAll`, `os.OpenFile`) is removed.

### `feeder_test.go` Updates

- Update `newFeeder` helper to accept and pass `*store.ExecutionStore` (use an in-memory SQLite DB the same way `execution_store_test.go` does, or pass nil and update tests accordingly)
- Add test cases covering:
  - Execution row created on each `processBeeGroup` call
  - `UpdatePID` called on successful bee start
  - Logs stored in `executions.logs` on completion
  - Status set to `failed` on bee error

## app.go Wire-up

`buildBee` receives `s.execStore` and passes it to `bee.NewFeeder`.

## Files Changed

| File | Change |
|------|--------|
| `internal/store/db.go` | Add migration 16 (drop + recreate executions) |
| `internal/model/execution.go` | `WorkerID` → `*string` with `omitempty` |
| `internal/store/execution_store.go` | Fix `Create` (`&workerID`); add `CreateBeeExecution`; `scanExecution` consistent with `*string` |
| `internal/store/execution_store_test.go` | Fix existing `WorkerID` comparison; add `CreateBeeExecution` test |
| `internal/bee/feeder.go` | Add `execStore` field; reorder `processBeeGroup`; rewrite `drainBeeOutput` |
| `internal/bee/feeder_test.go` | Update `newFeeder` helper; add execution lifecycle tests |
| `internal/app/app.go` | Pass `execStore` to `buildBee` / `bee.NewFeeder` |

## Out of Scope

- UI changes to surface bee executions separately from worker executions
- Periodic/streaming log flushes during execution
- Log size limits or truncation
