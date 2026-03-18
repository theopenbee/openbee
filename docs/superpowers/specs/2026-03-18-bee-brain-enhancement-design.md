# Bee Brain Enhancement Design

## Summary

Enhance the bee's capabilities as the system's brain by adding system status visibility, self-reflection, and a persistent memory system. This is achieved by adding 7 new MCP tools and 1 new database table, following the existing MCP tool architecture.

## Problem

The bee currently has limited ability to:

1. **Manage and improve itself** — cannot view its own execution history, reflect on performance, or learn from past interactions
2. **Monitor system state** — cannot check worker status, view execution logs, or get a system overview
3. **Remember across sessions** — no persistent memory for user preferences or accumulated experience

## Design

### Part 1: System Status Tools (Read-Only)

#### 1.1 `get_execution_logs`

View the last N lines of an execution's logs.

```
Parameters:
  - execution_id (string, required)
  - tail (int, optional, default 50)

Returns:
  {
    execution_id: string,
    worker_id: string | null,
    status: string,
    logs: string  // last N lines
  }
```

- For completed executions: read from `executions.logs` column, split by newline, return last N lines.
- For running executions: add a `GetExecutionLogs(executionID string) (string, error)` method on `worker.Manager`. This requires adding a `liveLogs map[string]*strings.Builder` field to the Manager struct, populated by `monitorExecution` as it reads process output. The method reads from this buffer for active processes. If the process is not tracked in memory, fall back to the DB `logs` column.

#### 1.2 `get_worker_status`

View a worker's current state including what it's doing right now.

```
Parameters:
  - worker_id (string, required)

Returns:
  {
    worker_id: string,
    name: string,
    status: string,           // "idle", "working", "error"
    current_execution: {      // null if idle
      id: string,
      task_id: string,
      instruction: string,
      started_at: int64 | null  // null if execution hasn't started yet
    } | null,
    pending_tasks_count: int   // counts tasks with status "pending" only
  }
```

- Joins `workers` table with running `executions` and pending `tasks` for the worker.

#### 1.3 `get_system_overview`

Aggregate system-wide statistics.

```
Parameters: none

Returns:
  {
    workers: {
      total: int,
      idle: int,
      working: int,
      error: int
    },
    tasks: {
      pending: int,
      running: int,
      completed: int,
      failed: int,
      cancelled: int,
      scheduled_active: int    // tasks with type="scheduled" and status="pending"
    },
    recent_executions: [
      {
        id: string,
        worker_name: string,
        status: string,
        started_at: int64 | null,
        completed_at: int64 | null
      }
    ]  // last 5 executions
  }
```

- Aggregate queries on `workers`, `tasks`, and `executions` tables.

#### 1.4 `list_bee_executions`

View the bee's own execution history for self-reflection.

```
Parameters:
  - limit (int, optional, default 10)

Returns:
  [
    {
      id: string,
      trigger_input: string,   // truncated to 200 chars
      status: string,
      started_at: int64,
      completed_at: int64,
      result: string           // truncated to 200 chars
    }
  ]
```

- Queries `executions` where `worker_id IS NULL`, ordered by `started_at DESC`.
- Note: only executions created after migration v16 (which made `worker_id` nullable) will appear.

### Part 2: Bee Self-Management

The bee runs in `~/.openbee/bee/` and can directly read/write its own `CLAUDE.md` file. No dedicated MCP tool is needed for persona modification — the bee uses its native file system access.

For self-reflection, the bee combines `list_bee_executions` and `get_execution_logs` to review its past performance, then stores insights via the memory system.

### Part 3: Memory System

#### 3.1 Database Schema

New table `bee_memories`:

```sql
CREATE TABLE bee_memories (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(scope, key)
);
```

- **scope = "global"**: Bee's accumulated global experience and knowledge
- **scope = {session_key}**: Per-user preferences and interaction patterns

#### 3.2 MCP Tools

##### `save_memory`

```
Parameters:
  - scope (string, required) — "global" or a session_key
  - key (string, required) — memory identifier
  - value (string, required) — memory content

Returns:
  { status: "saved" }
```

Upsert semantics: creates if key doesn't exist, updates if it does.

##### `get_memory`

```
Parameters:
  - scope (string, required)
  - key (string, optional)

Returns:
  - If key provided: single memory { key, value, updated_at } or null
  - If key omitted: array of memories in that scope [{ key, value, updated_at }], max 50 entries
```

##### `delete_memory`

```
Parameters:
  - scope (string, required)
  - key (string, required)

Returns:
  { status: "deleted" }
```

Deleting a non-existent key is a no-op (still returns `{ status: "deleted" }`).

### Part 4: Bee System Rules Update

Add memory usage guidelines to the bee's system rules (in `claudemd.go`):

```
## Memory Usage
- Before processing messages, load relevant memories:
  - get_memory(scope=session_key) for user-specific preferences
  - get_memory(scope="global") for accumulated experience
- When you discover user preferences, save them with save_memory
- After self-reflection, store conclusions as global memories
- Use descriptive keys, e.g. "user_language_preference", "task_assignment_insight"
```

## Code Changes

| File | Change |
|------|--------|
| `internal/mcp/tools.go` | Register 7 new tool definitions |
| `internal/mcp/server.go` | Add `ExecutionStore` and `MemoryStore` dependencies to `MCPServer` struct and `NewServer()` |
| `internal/mcp/handler.go` | Add handler cases for new tools |
| `internal/toolnames/toolnames.go` | Add constants for 7 new tool names |
| `internal/store/db.go` | Add `bee_memories` table migration |
| `internal/store/memory_store.go` | New file: CRUD operations for `bee_memories` |
| `internal/store/execution_store.go` | Add queries: bee executions, execution logs tail |
| `internal/store/task_store.go` | Add query: pending tasks count by worker |
| `internal/store/worker_store.go` | Add query: worker status aggregation |
| `internal/claudemd/claudemd.go` | Add memory usage guidelines to bee system rules |

## Not In Scope

- Worker management enhancements (current tools are sufficient)
- `update_bee_persona` tool (bee can edit its own CLAUDE.md directly)
- Automatic memory extraction (bee decides when to save memories via prompt guidance)
- Task retry logic, priority system, or inter-worker communication
