# Session Context Tools Design

**Date:** 2026-03-19
**Status:** Approved

## Overview

Add three MCP tools to expose `bee_session_contexts` to the AI brain, enabling fine-grained per-worker session control within a conversation session.

## Background

`bee_session_contexts` stores Claude Code session IDs keyed by `(session_key, agent_id)`. Each row represents one agent's conversation continuity in one platform session:

- `agent_id = "bee"` — the bee brain's own Claude session
- `agent_id = <worker_uuid>` — a worker's Claude session for that conversation

Workers write their row after completing an `immediate`-type task (dispatcher). Bee writes its row after processing a message batch (feeder). These session IDs are passed as `--resume <session_id>` to maintain conversation memory across invocations.

## Requirements

1. **Query linked workers** — the AI brain can list which workers (and bee) have active session contexts for a given session key.
2. **Confirmation on bulk clear** — `clear_session` must prompt for confirmation (two-step) when more than one worker has an active session context.
3. **Clear single worker** — the AI brain can reset one worker's session context without affecting others.

## Tools

### 1. `get_session_context`

Lists all agents with active session contexts for a session key.

**Input schema**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `session_key` | string | yes | The session key to query |

**Output** — array of agent entries, ordered by `updated_at DESC`:

```json
[
  {
    "agent_id": "bee",
    "type": "bee",
    "name": "bee",
    "updated_at": 1742380000000
  },
  {
    "agent_id": "a1b2-c3d4-...",
    "type": "worker",
    "name": "天天",
    "updated_at": 1742381000000
  }
]
```

- Returns `[]` when the session has no contexts (new session).
- If a worker has been deleted but its session row still exists, `name` is `"(deleted)"`.

---

### 2. `clear_session` (modified)

Existing tool; adds an optional `force` parameter to support two-step confirmation.

**Input schema**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `session_key` | string | yes | The session key to clear |
| `force` | boolean | no (default: false) | Skip confirmation check and clear unconditionally |

**Behavior**

- Count workers (`agent_id != "bee"`) in `bee_session_contexts` for `session_key`.
- If worker count > 1 **and** `force` is false/absent → return confirmation-required response (no side effects).
- Otherwise (worker count ≤ 1, or `force=true`) → execute existing clear logic unchanged.

**Output A — confirmation required** (worker count > 1, force=false):

```json
{
  "requires_confirmation": true,
  "worker_count": 2,
  "linked_workers": [
    { "worker_id": "a1b2-...", "name": "天天" },
    { "worker_id": "e5f6-...", "name": "小王" }
  ],
  "message": "此会话链接了 2 位员工，清空将重置所有员工和 bee 的对话上下文。请确认后以 force=true 重新调用。"
}
```

**Output B — cleared**:

```json
{
  "cancelled_tasks": 3,
  "cleared": true
}
```

---

### 3. `clear_worker_session`

Resets one worker's Claude session context within a session, without affecting other workers or bee.

**Input schema**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `session_key` | string | yes | The session key |
| `worker_id` | string | yes | Worker ID whose session context to delete |

**Output**:

```json
{
  "cleared": true,
  "worker_id": "a1b2-c3d4-...",
  "worker_name": "天天"
}
```

- Idempotent: returns `cleared: true` even if no row existed.
- Returns error if `worker_id = "bee"`: `"cannot clear bee session context with this tool, use clear_session instead"`.
- Does **not** cancel tasks; if task cancellation is also needed, the caller combines this with `cancel_task`.

---

## Implementation

### `internal/store/session_store.go`

Add `SessionAgent` struct and two new methods.

```go
type SessionAgent struct {
    AgentID   string
    AgentType string // "bee" or "worker"
    Name      string
    UpdatedAt int64
}

// ListSessionContexts returns all agents with session contexts for sessionKey,
// enriched with worker names via LEFT JOIN.
func (s *SessionStore) ListSessionContexts(ctx context.Context, sessionKey string) ([]SessionAgent, error)

// DeleteWorkerSessionContext removes the session context row for one worker.
// Deleting a non-existent row is not an error.
func (s *SessionStore) DeleteWorkerSessionContext(ctx context.Context, sessionKey, workerID string) error
```

SQL for `ListSessionContexts`:
```sql
SELECT sc.agent_id, sc.updated_at, w.name
FROM bee_session_contexts sc
LEFT JOIN bee_workers w ON w.id = sc.agent_id
WHERE sc.session_key = ?
ORDER BY sc.updated_at DESC
```

### `internal/toolnames/toolnames.go`

```go
GetSessionContext    = "get_session_context"
ClearWorkerSession   = "clear_worker_session"
```

### `internal/mcp/server.go`

- Add `sessionStore *store.SessionStore` field to `MCPServer` (consistent with how `workerStore`, `taskStore`, etc. are held directly).
- Update `NewServer` signature to accept `*store.SessionStore`.

### `internal/mcp/tools.go`

- Add two new `toolSchema` entries in `toolSchemas()`.
- Add two new handler methods: `toolGetSessionContext`, `toolClearWorkerSession`.
- Modify `toolClearSession` to check worker count when `force` is false.
- Add cases in `callTool` switch.

### `internal/task_dispatcher/dispatcher.go`

Extend the local `SessionStore` interface:

```go
type SessionStore interface {
    GetSessionContext(ctx context.Context, sessionKey, agentID string) (string, error)
    UpsertSessionContext(ctx context.Context, sessionKey, agentID, sessionID string) error
    ClearSessionContexts(ctx context.Context, sessionKey string) error
    ListSessionContexts(ctx context.Context, sessionKey string) ([]store.SessionAgent, error)   // new
    DeleteWorkerSessionContext(ctx context.Context, sessionKey, workerID string) error           // new
}
```

## Files Changed

| File | Change |
|------|--------|
| `internal/store/session_store.go` | Add `SessionAgent`, `ListSessionContexts`, `DeleteWorkerSessionContext` |
| `internal/toolnames/toolnames.go` | Add `GetSessionContext`, `ClearWorkerSession` constants |
| `internal/mcp/server.go` | Add `sessionStore` field + update `NewServer` |
| `internal/mcp/tools.go` | Add 2 tool schemas, 2 handlers, modify `toolClearSession` |
| `internal/task_dispatcher/dispatcher.go` | Extend `SessionStore` interface |

## Error Handling

- `get_session_context`: DB error → return tool error.
- `clear_session` (confirmation path): DB error on worker count query → fail safe, return error (do not proceed with clear).
- `clear_worker_session`: `worker_id="bee"` → return explicit error; DB error → return tool error.

## Testing

- Unit tests for `ListSessionContexts` and `DeleteWorkerSessionContext` in `store/` package.
- Unit tests for all three tool handlers in `mcp/` package (using existing fake store pattern).
- Table-driven cases for `clear_session`: (0 workers, no force), (1 worker, no force), (2 workers, no force → confirm), (2 workers, force=true → clear).
