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

### 1. `list_session_contexts`

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

- Use `s.sessionStore.ListSessionContexts(sessionKey)` to count workers (`type == "worker"`).
- If worker count > 1 **and** `force` is false/absent → return confirmation-required response (no side effects).
  - The threshold is `> 1` (not `>= 1`) because a single-worker session is considered low-risk; the protection targets sessions with multiple workers where a bulk reset has significant impact.
- Otherwise (worker count ≤ 1, or `force=true`) → execute existing clear logic unchanged, which calls `s.sessionClearer.ClearSession()` for the actual clearing.
- DB error on the count query → fail safe, return error (do not proceed with clear).

**Output shape is identical to the existing implementation** — unchanged in the non-confirmation path.

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

**Output B — cleared** (same as current implementation):

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

- Idempotent: returns `cleared: true` even if no session row existed for this worker.
- `worker_name` is always resolved from `bee_workers` regardless of whether a session row existed: actual name if the worker exists, `"(deleted)"` if the worker has been deleted. This allows the caller to confirm which worker was targeted even on a no-op delete.
- Returns error if `worker_id` is empty or equals `"bee"`: `"cannot clear bee session context with this tool, use clear_session instead"`.
- Does **not** cancel tasks; if task cancellation is also needed, the caller combines this with `cancel_task`.

---

## Implementation

### `internal/store/session_store.go`

Add `SessionAgent` struct and two new methods.

```go
type SessionAgent struct {
    AgentID   string
    AgentType string // "bee" or "worker"; derived in Go: BeeAgentID → "bee", else "worker"
    Name      string // worker name from bee_workers, "bee" for bee, "(deleted)" for deleted workers
    UpdatedAt int64
}

// ListSessionContexts returns all agents with session contexts for sessionKey,
// enriched with worker names via LEFT JOIN.
func (s *SessionStore) ListSessionContexts(ctx context.Context, sessionKey string) ([]SessionAgent, error)

// DeleteWorkerSessionContext removes the session context row for one worker.
// Deleting a non-existent row is not an error.
func (s *SessionStore) DeleteWorkerSessionContext(ctx context.Context, sessionKey, workerID string) error
```

SQL for `ListSessionContexts` (name resolved in SQL via COALESCE; AgentType derived in Go):
```sql
SELECT sc.agent_id, sc.updated_at,
       COALESCE(w.name, CASE WHEN sc.agent_id = 'bee' THEN 'bee' ELSE '(deleted)' END) AS name
FROM bee_session_contexts sc
LEFT JOIN bee_workers w ON w.id = sc.agent_id
WHERE sc.session_key = ?
ORDER BY sc.updated_at DESC
```

`AgentType` is not a DB column; set in Go after scanning: `if row.AgentID == BeeAgentID { row.AgentType = "bee" } else { row.AgentType = "worker" }`.

No schema migration needed — no new tables or columns.

### `internal/toolnames/toolnames.go`

```go
ListSessionContexts = "list_session_contexts"
ClearWorkerSession  = "clear_worker_session"
```

### `internal/mcp/server.go`

- Add `sessionStore *store.SessionStore` field to `MCPServer` (consistent with `workerStore`, `taskStore`, etc.).
- Update `NewServer` signature to accept `*store.SessionStore` as a new parameter.
- **All existing `setupMCP*` test helpers in `tools_test.go` must be updated** to pass `store.NewSessionStore(db)`.

### `internal/mcp/tools.go`

- Add two new `toolSchema` entries in `toolSchemas()` — total schema count becomes **20** (update the count assertion in `TestToolSchemas_Count_AfterNewTools`).
- Add two new handler methods: `toolListSessionContexts`, `toolClearWorkerSession`.
- Modify `toolClearSession`:
  - Parse new optional `Force bool` from args.
  - **Update the `clear_session` JSON schema** in `toolSchemas()` to advertise the `force` boolean field so the LLM sees it in `tools/list`.
  - If `!force`: call `s.sessionStore.ListSessionContexts(sessionKey)` to count workers. On DB error, return error. If worker count > 1, return confirmation response.
  - Then proceed with existing logic (`s.sessionClearer.ClearSession(...)` is unchanged).
- Add two new cases in `callTool` switch.

### `internal/task_dispatcher/dispatcher.go`

**No changes.** The dispatcher's `SessionStore` interface is a minimal interface for methods the dispatcher actually calls. The two new store methods are consumed only by MCP tool handlers (via the concrete `*store.SessionStore`), so no interface extension here.

## Files Changed

| File | Change |
|------|--------|
| `internal/store/session_store.go` | Add `SessionAgent`, `ListSessionContexts`, `DeleteWorkerSessionContext` |
| `internal/toolnames/toolnames.go` | Add `ListSessionContexts`, `ClearWorkerSession` constants |
| `internal/mcp/server.go` | Add `sessionStore` field + update `NewServer` |
| `internal/mcp/tools.go` | Add 2 tool schemas, 2 handlers, modify `toolClearSession` |
| `internal/mcp/tools_test.go` | Update 3 `setupMCP*` helpers + update schema count (18 → 20) + new tests |

## Error Handling

- `list_session_contexts`: DB error → return tool error.
- `clear_session` (confirmation path): DB error on worker count query → fail safe, return error (do not proceed with clear).
- `clear_worker_session`: empty `worker_id` or `worker_id="bee"` → return explicit error; DB error → return tool error.

## Testing

- Unit tests for `ListSessionContexts` and `DeleteWorkerSessionContext` in `store/` package.
- Unit tests for all three tool handlers in `mcp/` package (using existing fake store pattern).
- Update `TestToolSchemas_Count_AfterNewTools`: expected count `18 → 20`.
- Update all three `setupMCP*` test helpers to pass `store.NewSessionStore(db)`.
- Table-driven cases for `clear_session`:
  - 0 workers, no force → cleared
  - 1 worker, no force → cleared
  - 2 workers, no force → requires_confirmation
  - 2 workers, force=true → cleared
