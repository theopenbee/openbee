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
- No migration compatibility needed — project is pre-release, existing data is discarded.

## Schema Change

Modify migration version 2 in `internal/store/db.go` to make `worker_id` nullable and remove the foreign key constraint:

```sql
CREATE TABLE IF NOT EXISTS executions (
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
)
```

No new migration needed — the existing migration is edited directly.

## ExecutionStore

Add one new method to `internal/store/execution_store.go`:

```go
func (s *ExecutionStore) CreateBeeExecution(sessionID, triggerInput string) (model.WorkerExecution, error)
```

Inserts a row with `worker_id = NULL`. All other update methods (`UpdatePID`, `UpdateResult`, `UpdateLogs`) are reused as-is.

## Feeder Changes (`internal/bee/feeder.go`)

### Struct & Constructor

Add `execStore *store.ExecutionStore` field to `Feeder`. Update `NewFeeder` signature accordingly.

### `processBeeGroup` Lifecycle

1. Create execution record before bee starts → get `execID`
2. Run bee via `f.runner.Run(...)`
3. On successful process start: `execStore.UpdatePID(execID, pid)` sets status → `running`
4. Call `drainBeeOutput(outputCh, execID)` — accumulates logs in a `strings.Builder`
5. On success: `execStore.UpdateResult(execID, logs, model.ExecStatusCompleted)`
6. On failure: `execStore.UpdateResult(execID, logs+"\n"+errMsg, model.ExecStatusFailed)`

### `drainBeeOutput` Signature Change

```go
// Before
func (f *Feeder) drainBeeOutput(ch <-chan claude.Output, sessionID string) error

// After
func (f *Feeder) drainBeeOutput(ch <-chan claude.Output, execID string) (string, error)
```

Returns accumulated log string. All file I/O (`os.MkdirAll`, `os.OpenFile`) is removed.

## app.go Wire-up

`buildBee` receives `s.execStore` and passes it to `bee.NewFeeder`.

## Out of Scope

- UI changes to surface bee executions separately from worker executions
- Periodic/streaming log flushes during execution
- Log size limits or truncation
