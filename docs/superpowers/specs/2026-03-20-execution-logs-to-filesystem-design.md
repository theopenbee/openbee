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

**Migration 20** — drop `logs` column (SQLite requires table rebuild):
```sql
DROP TABLE IF EXISTS bee_executions;
CREATE TABLE bee_executions (
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
CREATE INDEX idx_executions_worker_id ON bee_executions(worker_id);
CREATE INDEX idx_executions_session_id ON bee_executions(session_id);
```

### 2. Model

`internal/model/execution.go`: replace `Logs string` with `LogPath string`.

### 3. Config

`internal/config/config.go`: add `DefaultLogsDir()` returning `~/.openbee/logs`.

### 4. ExecutionStore

`internal/store/execution_store.go`:

- Remove `UpdateLogs(id, logs string)`
- Add `WriteLog(id string, startedAt int64, content string) (logPath string, error)`
  - Computes date from `startedAt`: `~/.openbee/logs/<YYYY-MM-DD>/`
  - Creates directory if not exists
  - Writes content to `<logsDir>/<YYYY-MM-DD>/<execution_id>.log` (overwrite)
  - Updates `bee_executions SET log_path=? WHERE id=?`
  - Returns the log path written
- Update `execSelect` and `scanExecution` to use `log_path` instead of `logs`
- Remove `GetLogsByID` (superseded by reading `LogPath` from `GetByID`)

### 5. Callers

**`internal/bee/feeder.go`**: Replace `execStore.UpdateLogs(exec.ID, logs)` with `execStore.WriteLog(exec.ID, *exec.StartedAt, logs)`.

**`internal/worker/manager.go`**: Same replacement in `OutputDone` and `OutputError` branches.

### 6. MCP Tool Removal

- `internal/toolnames/toolnames.go`: remove `GetExecutionLogs` constant
- `internal/mcp/tools.go`: remove `toolGetExecutionLogs` method and its registration in `NewMCPServer`
- `internal/mcp/server.go`: no changes needed if tool is deregistered in `tools.go`

### 7. API Endpoint Removal

`internal/api/router.go`: remove `GET /executions/:id/logs` route and the `streamLogs` handler in `execution_handler.go`.

### 8. claudemd Update

`internal/claudemd/bee.go`: in the status tools section:
- Remove `get_execution_logs` from the tool list
- Add guidance: "查看执行日志时，从执行记录的 `log_path` 字段获取文件路径，然后直接读取该文件"

## File Changelist

| File | Change |
|------|--------|
| `internal/store/db.go` | Add migrations 19 & 20 |
| `internal/model/execution.go` | `Logs` → `LogPath` |
| `internal/config/config.go` | Add `DefaultLogsDir()` |
| `internal/store/execution_store.go` | Replace `UpdateLogs` with `WriteLog`; update select/scan; remove `GetLogsByID` |
| `internal/bee/feeder.go` | Use `WriteLog` |
| `internal/worker/manager.go` | Use `WriteLog` |
| `internal/toolnames/toolnames.go` | Remove `GetExecutionLogs` |
| `internal/mcp/tools.go` | Remove `toolGetExecutionLogs` and registration |
| `internal/api/router.go` | Remove logs WebSocket route |
| `internal/api/execution_handler.go` | Remove `streamLogs` handler |
| `internal/claudemd/bee.go` | Remove tool ref; add file-read guidance |
| Tests | Update `execution_store_test.go`, `feeder_test.go` |

## Out of Scope

- Log rotation / cleanup of old log files
- Live log streaming (WebSocket removed; future work if needed)
- Compressing log files
