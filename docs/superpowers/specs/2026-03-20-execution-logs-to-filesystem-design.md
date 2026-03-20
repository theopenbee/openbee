# Execution Logs: DB → Filesystem Migration

**Date:** 2026-03-20
**Status:** Approved

## Problem

Execution logs for both bee and workers are stored in `bee_executions.logs` (SQLite TEXT column). This causes:
- Large DB rows for long-running executions
- AI cannot read logs without a dedicated MCP tool
- Logs are not inspectable with standard file tools

## Goal

Move log storage to `~/.openbee/logs/<YYYY-MM-DD>/<execution_id>.log`, remove the `logs` DB column, add a `log_path` column, and let AI read log files directly rather than through a tool.

## Design

### 1. Database Schema

Two new migrations (versions 19 and 20):

**Migration 19** — add `log_path` column:
```sql
ALTER TABLE bee_executions ADD COLUMN log_path TEXT NOT NULL DEFAULT '';
```

**Migration 20** — drop `logs` column using SQLite rename pattern (preserves existing rows):
```sql
CREATE TABLE bee_executions_new (
    id             TEXT PRIMARY KEY,
    worker_id      TEXT,
    session_id     TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending',
    ai_process_pid INTEGER NOT NULL DEFAULT 0,
    trigger_input  TEXT NOT NULL DEFAULT '',
    result         TEXT NOT NULL DEFAULT '',
    log_path       TEXT NOT NULL DEFAULT '',
    started_at     INTEGER,
    completed_at   INTEGER
);
INSERT INTO bee_executions_new
    SELECT id, worker_id, session_id, status, ai_process_pid,
           trigger_input, result, log_path, started_at, completed_at
    FROM bee_executions;
DROP TABLE bee_executions;
ALTER TABLE bee_executions_new RENAME TO bee_executions;
CREATE INDEX idx_executions_worker_id ON bee_executions(worker_id);
CREATE INDEX idx_executions_session_id ON bee_executions(session_id);
```

Note: do **not** use `IF NOT EXISTS` on these index creations — the old indexes are dropped with the old table in the same migration, so they will not exist at this point.


### 2. Model

`internal/model/execution.go`: replace `Logs string` with `LogPath string`.

### 3. Config

`internal/config/config.go`: add `DefaultLogsDir()` returning `~/.openbee/logs`.

### 4. ExecutionStore

`internal/store/execution_store.go`:

- Remove `UpdateLogs(id, logs string)`
- Add `WriteLog(id string, startedAt *int64, content string) (logPath string, error)`
  - Accepts `*int64` to match `model.WorkerExecution.StartedAt`; falls back to `time.Now()` if nil
  - Computes date from startedAt: `~/.openbee/logs/<YYYY-MM-DD>/`
  - Creates directory if not exists (`os.MkdirAll`)
  - Writes the full `content` to `<logsDir>/<YYYY-MM-DD>/<execution_id>.log` — single overwrite, not append (callers accumulate the full log before calling)
  - Updates `bee_executions SET log_path=? WHERE id=?`
  - Returns the log path written
- Update `execSelect` and `scanExecution` to use `log_path` instead of `logs`
- Remove `GetLogsByID` (superseded by reading `LogPath` from `GetByID`)

### 5. Callers

Both callers already accumulate the complete log string before calling `UpdateLogs`, so the single-overwrite behavior is correct.

**`internal/bee/feeder.go`**: Replace `execStore.UpdateLogs(exec.ID, logs)` with `execStore.WriteLog(exec.ID, exec.StartedAt, logs)`.

**`internal/worker/manager.go`**: Same replacement in both `OutputDone` and `OutputError` branches. `exec.StartedAt` is populated by `ExecutionStore.Create` before the monitor goroutine runs.

### 6. MCP Tool Removal

- `internal/toolnames/toolnames.go`: remove `GetExecutionLogs` constant
- `internal/mcp/tools.go`: remove `toolGetExecutionLogs` method and its registration in `NewMCPServer`

### 7. Subscriber Infrastructure Removal

With the WebSocket log-streaming endpoint removed, the live-log infrastructure in `manager.go` becomes dead code and should be cleaned up:

- Remove `logSubscribers map[string][]chan claude.Output` field and all references
- Remove `liveLogSnapshots map[string]string` field and all references
- Remove `SubscribeLogs(executionID string)` method
- Remove `GetExecutionLogs(executionID string)` method

This is safe because the only consumer was `streamLogs` in `execution_handler.go` and `toolGetExecutionLogs` in `mcp/tools.go`, both of which are being deleted.

### 8. API Endpoint Removal

`internal/api/router.go`: remove `GET /executions/:id/logs` route.
`internal/api/execution_handler.go`: remove `streamLogs` handler.

### 9. claudemd Update

`internal/claudemd/bee.go` has two references to `get_execution_logs` to remove:

1. The tool list entry (via `toolnames.GetExecutionLogs` constant) — delete the bullet line
2. The hardcoded prose in "使用场景": `用 get_execution_logs 查看详情` — rewrite to say "直接读取 `log_path` 文件查看详情"

Add guidance: "查看执行日志时，从执行记录的 `log_path` 字段获取文件路径，然后直接读取该文件"

## File Changelist

| File | Change |
|------|--------|
| `internal/store/db.go` | Add migrations 19 & 20 |
| `internal/model/execution.go` | `Logs` → `LogPath` |
| `internal/config/config.go` | Add `DefaultLogsDir()` |
| `internal/store/execution_store.go` | Replace `UpdateLogs` with `WriteLog`; update select/scan; remove `GetLogsByID` |
| `internal/bee/feeder.go` | Use `WriteLog` |
| `internal/worker/manager.go` | Use `WriteLog`; remove subscriber infrastructure |
| `internal/toolnames/toolnames.go` | Remove `GetExecutionLogs` |
| `internal/mcp/tools.go` | Remove `toolGetExecutionLogs` and registration |
| `internal/mcp/tools_test.go` | Remove `TestCallTool_GetExecutionLogs` test |
| `internal/api/router.go` | Remove logs WebSocket route |
| `internal/api/execution_handler.go` | Remove `streamLogs` handler |
| `internal/claudemd/bee.go` | Remove tool ref; add file-read guidance |
| `internal/store/execution_store_test.go` | Remove `TestExecutionStore_GetLogsByID` and `UpdateLogs` tests; add `WriteLog` test |
| `internal/bee/feeder_test.go` | Update log assertions to use `log_path` |

## Out of Scope

- Log rotation / cleanup of old log files
- Live log streaming (WebSocket removed; future work if needed)
- Compressing log files
