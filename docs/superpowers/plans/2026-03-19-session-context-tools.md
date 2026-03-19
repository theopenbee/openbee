# Session Context MCP Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add three MCP tools (`list_session_contexts`, `clear_worker_session`, modified `clear_session`) so the AI brain can query and manage per-worker session contexts with two-step confirmation for bulk clears.

**Architecture:** Extend `SessionStore` with two new methods backed by the existing `bee_session_contexts` table (no migration needed). Inject `*store.SessionStore` directly into `MCPServer` (consistent with existing store injection pattern). The two new tools and the modified `clear_session` handler all operate on this store reference.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), standard `database/sql`, existing `store` / `mcp` / `toolnames` packages.

---

## File Map

| File | Role |
|------|------|
| `internal/store/session_store.go` | Add `SessionAgent` struct + `ListSessionContexts` + `DeleteWorkerSessionContext` |
| `internal/store/session_store_test.go` | Tests for the two new store methods |
| `internal/toolnames/toolnames.go` | Two new tool-name constants |
| `internal/mcp/server.go` | Add `sessionStore` field; update `NewServer` signature |
| `internal/mcp/tools.go` | Two new schemas + two new handlers + modified `toolClearSession` |
| `internal/mcp/tools_test.go` | Fix three `setupMCP*` helpers; update schema-count assertion; add new tool tests |

---

## Task 1: Store layer — `ListSessionContexts` + `DeleteWorkerSessionContext`

**Files:**
- Modify: `internal/store/session_store.go`
- Modify: `internal/store/session_store_test.go`

### Step 1.1 — Write failing tests for `ListSessionContexts`

Open `internal/store/session_store_test.go` and append:

```go
func TestSessionStore_ListSessionContexts_Empty(t *testing.T) {
	_, ss := setupSessionDB(t)
	got, err := ss.ListSessionContexts(context.Background(), "no-such-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestSessionStore_ListSessionContexts_BeeAndWorker(t *testing.T) {
	db, ss := setupSessionDB(t)
	ctx := context.Background()

	// Insert a worker so the LEFT JOIN can resolve its name.
	// WorkerStore.Create takes model.Worker directly (no WorkerParams struct).
	ws := store.NewWorkerStore(db)
	w, err := ws.Create(model.Worker{Name: "天天", WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	ss.UpsertSessionContext(ctx, "sk", store.BeeAgentID, "bee-sid")   //nolint:errcheck
	ss.UpsertSessionContext(ctx, "sk", w.ID, "worker-sid")            //nolint:errcheck

	got, err := ss.ListSessionContexts(ctx, "sk")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(got), got)
	}

	byAgent := make(map[string]store.SessionAgent)
	for _, a := range got {
		byAgent[a.AgentID] = a
	}

	bee := byAgent[store.BeeAgentID]
	if bee.AgentType != "bee" || bee.Name != "bee" {
		t.Errorf("bee entry: got type=%q name=%q", bee.AgentType, bee.Name)
	}

	wkr := byAgent[w.ID]
	if wkr.AgentType != "worker" || wkr.Name != "天天" {
		t.Errorf("worker entry: got type=%q name=%q", wkr.AgentType, wkr.Name)
	}
}

func TestSessionStore_ListSessionContexts_DeletedWorker(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	// Insert a session context for a worker UUID that does not exist in bee_workers.
	ss.UpsertSessionContext(ctx, "sk", "ghost-worker-id", "sid") //nolint:errcheck

	got, err := ss.ListSessionContexts(ctx, "sk")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].Name != "(deleted)" {
		t.Errorf("expected (deleted), got %q", got[0].Name)
	}
	if got[0].AgentType != "worker" {
		t.Errorf("expected type=worker, got %q", got[0].AgentType)
	}
}
```

- [ ] **Step 1.2 — Run tests to verify they fail**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/store/... -run "TestSessionStore_ListSessionContexts" -v
```

Expected: FAIL — `ss.ListSessionContexts undefined`

- [ ] **Step 1.3 — Write failing test for `DeleteWorkerSessionContext`**

Append to `internal/store/session_store_test.go`:

```go
func TestSessionStore_DeleteWorkerSessionContext_Basic(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "sk", store.BeeAgentID, "bee-sid")   //nolint:errcheck
	ss.UpsertSessionContext(ctx, "sk", "worker-1", "w1-sid")          //nolint:errcheck

	if err := ss.DeleteWorkerSessionContext(ctx, "sk", "worker-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// worker-1 should be gone; bee should remain.
	w1, _ := ss.GetSessionContext(ctx, "sk", "worker-1")
	if w1 != "" {
		t.Errorf("expected worker-1 cleared, got %q", w1)
	}
	bee, _ := ss.GetSessionContext(ctx, "sk", store.BeeAgentID)
	if bee != "bee-sid" {
		t.Errorf("expected bee unaffected, got %q", bee)
	}
}

func TestSessionStore_DeleteWorkerSessionContext_Idempotent(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	// Delete a row that doesn't exist — must not error.
	if err := ss.DeleteWorkerSessionContext(ctx, "sk", "nobody"); err != nil {
		t.Errorf("expected no error on missing row, got %v", err)
	}
}
```

- [ ] **Step 1.4 — Run test to verify it fails**

```bash
go test ./internal/store/... -run "TestSessionStore_DeleteWorkerSessionContext" -v
```

Expected: FAIL — `ss.DeleteWorkerSessionContext undefined`

- [ ] **Step 1.5 — Implement store methods**

In `internal/store/session_store.go`, add after the existing constants/types:

```go
// SessionAgent represents one agent's session context entry, enriched with
// a human-readable name.
type SessionAgent struct {
	AgentID   string
	AgentType string // "bee" or "worker"
	Name      string // worker name, "bee", or "(deleted)"
	UpdatedAt int64
}

// ListSessionContexts returns all agents with session contexts for sessionKey,
// ordered by updated_at DESC. Worker names are resolved via LEFT JOIN; deleted
// workers appear as "(deleted)". AgentType is derived in Go from AgentID.
func (s *SessionStore) ListSessionContexts(ctx context.Context, sessionKey string) ([]SessionAgent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sc.agent_id, sc.updated_at,
		       COALESCE(w.name, CASE WHEN sc.agent_id = 'bee' THEN 'bee' ELSE '(deleted)' END) AS name
		FROM bee_session_contexts sc
		LEFT JOIN bee_workers w ON w.id = sc.agent_id
		WHERE sc.session_key = ?
		ORDER BY sc.updated_at DESC`,
		sessionKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SessionAgent
	for rows.Next() {
		var a SessionAgent
		if err := rows.Scan(&a.AgentID, &a.UpdatedAt, &a.Name); err != nil {
			return nil, err
		}
		if a.AgentID == BeeAgentID {
			a.AgentType = "bee"
		} else {
			a.AgentType = "worker"
		}
		result = append(result, a)
	}
	if result == nil {
		result = []SessionAgent{}
	}
	return result, rows.Err()
}

// DeleteWorkerSessionContext removes the session context row for one worker.
// Deleting a non-existent row is not an error.
func (s *SessionStore) DeleteWorkerSessionContext(ctx context.Context, sessionKey, workerID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM bee_session_contexts WHERE session_key = ? AND agent_id = ?`,
		sessionKey, workerID,
	)
	return err
}
```

Note: the `session_store_test.go` file imports `"github.com/theopenbee/openbee/internal/store"` but not `model`. The new `TestSessionStore_ListSessionContexts_BeeAndWorker` test uses `model.Worker{...}`, so you must add `"github.com/theopenbee/openbee/internal/model"` to the import block at the top of the test file.

- [ ] **Step 1.6 — Check the worker creation API used in tests**

```bash
grep -n "func.*Create" internal/store/worker_store.go | head -10
```

If `ws.Create` takes different args, update the test in Step 1.1 to match.

- [ ] **Step 1.7 — Run all store tests**

```bash
go test ./internal/store/... -v
```

Expected: All PASS

- [ ] **Step 1.8 — Commit**

```bash
git add internal/store/session_store.go internal/store/session_store_test.go
git commit -m "feat: add ListSessionContexts and DeleteWorkerSessionContext to SessionStore"
```

---

## Task 2: Add tool-name constants

**Files:**
- Modify: `internal/toolnames/toolnames.go`

- [ ] **Step 2.1 — Add constants**

In `internal/toolnames/toolnames.go`, append to the `const` block:

```go
ListSessionContexts = "list_session_contexts"
ClearWorkerSession  = "clear_worker_session"
```

- [ ] **Step 2.2 — Verify compilation**

```bash
go build ./internal/toolnames/...
```

Expected: no errors

- [ ] **Step 2.3 — Commit**

```bash
git add internal/toolnames/toolnames.go
git commit -m "feat: add ListSessionContexts and ClearWorkerSession tool-name constants"
```

---

## Task 3: Inject `sessionStore` into `MCPServer` + fix test helpers

**Files:**
- Modify: `internal/mcp/server.go`
- Modify: `internal/mcp/tools_test.go`

- [ ] **Step 3.1 — Update `MCPServer` struct and `NewServer`**

In `internal/mcp/server.go`:

1. Add `sessionStore *store.SessionStore` field to the `MCPServer` struct after `memoryStore`:

```go
type MCPServer struct {
    workerStore    *store.WorkerStore
    manager        *worker.Manager
    taskStore      *store.TaskStore
    messageStore   *store.MessageStore
    senders        map[string]platform.PlatformSenderAdapter
    execStopper    ExecutionStopper
    sessionClearer SessionClearer
    executionStore *store.ExecutionStore
    memoryStore    *store.MemoryStore
    sessionStore   *store.SessionStore   // NEW

    mu       sync.Mutex
    sessions map[string]chan rpcResponse
}
```

2. Add `sessionStore *store.SessionStore` parameter to `NewServer` (add after `memStore`):

```go
func NewServer(
    ws *store.WorkerStore,
    mgr *worker.Manager,
    ts *store.TaskStore,
    ms *store.MessageStore,
    senders map[string]platform.PlatformSenderAdapter,
    execStopper ExecutionStopper,
    sessionClearer SessionClearer,
    es *store.ExecutionStore,
    memStore *store.MemoryStore,
    sessionStore *store.SessionStore,
) *MCPServer {
    return &MCPServer{
        workerStore:    ws,
        manager:        mgr,
        taskStore:      ts,
        messageStore:   ms,
        senders:        senders,
        execStopper:    execStopper,
        sessionClearer: sessionClearer,
        executionStore: es,
        memoryStore:    memStore,
        sessionStore:   sessionStore,
        sessions:       make(map[string]chan rpcResponse),
    }
}
```

- [ ] **Step 3.2 — Fix the three test helpers in `tools_test.go`**

The current helpers call `mcp.NewServer(...)` with 9 args. Add `store.NewSessionStore(db)` as the 10th argument in all three:

`setupMCPServerWithMessaging` (line ~38):
```go
return mcp.NewServer(ws, mgr, ts, ms, senders, nil, nil, es, store.NewMemoryStore(db), store.NewSessionStore(db))
```

`setupMCPServerWithSender` (line ~217):
```go
return mcp.NewServer(ws, mgr, ts, ms, senders, nil, nil, es, store.NewMemoryStore(db), store.NewSessionStore(db)), db
```

`setupMCPServerWithClear` (line ~459):
```go
return mcp.NewServer(ws, mgr, ts, ms, senders, stopper, clearer, es, store.NewMemoryStore(db), store.NewSessionStore(db)), db, stopper, clearer
```

- [ ] **Step 3.3 — Find and fix the production call site for `NewServer`**

```bash
grep -rn "mcp.NewServer\|mcp\.NewServer" --include="*.go" .
```

Update the production call site (likely in `cmd/` or `internal/app/`) to pass `store.NewSessionStore(db)` as the new final argument.

- [ ] **Step 3.4 — Verify compilation**

```bash
go build ./...
```

Expected: no errors

- [ ] **Step 3.5 — Run existing tests to confirm nothing broke**

```bash
go test ./internal/mcp/... -v
```

Expected: All existing tests PASS

- [ ] **Step 3.6 — Commit**

```bash
git add internal/mcp/server.go internal/mcp/tools_test.go
git add $(git diff --name-only HEAD -- cmd/ internal/app/ | tr '\n' ' ')
git commit -m "feat: inject sessionStore into MCPServer"
```

---

## Task 4: Add `list_session_contexts` tool

**Files:**
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/tools_test.go`

- [ ] **Step 4.1 — Write failing tests**

Append to `internal/mcp/tools_test.go`:

```go
// --- list_session_contexts ---

func TestCallTool_ListSessionContexts_Empty(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	result, err := s.CallTool("list_session_contexts", mustMarshal(t, map[string]any{
		"session_key": "no-such-session",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	agents, ok := result.([]store.SessionAgent)
	if !ok {
		t.Fatalf("expected []store.SessionAgent, got %T", result)
	}
	if len(agents) != 0 {
		t.Errorf("expected empty slice, got %d", len(agents))
	}
}

func TestCallTool_ListSessionContexts_MissingSessionKey(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool("list_session_contexts", mustMarshal(t, map[string]any{}))
	if err == nil {
		t.Error("expected error for missing session_key")
	}
}
```

- [ ] **Step 4.2 — Run tests to verify they fail**

```bash
go test ./internal/mcp/... -run "TestCallTool_ListSessionContexts" -v
```

Expected: FAIL — `unknown tool: list_session_contexts`

- [ ] **Step 4.3 — Add schema entry in `toolSchemas()`**

In `internal/mcp/tools.go`, append a new entry to the `toolSchemas()` return slice:

```go
{
    Name:        toolnames.ListSessionContexts,
    Description: "List all agents (bee and workers) that have active session contexts for a given session key.",
    InputSchema: map[string]any{
        "type":     "object",
        "required": []string{"session_key"},
        "properties": map[string]any{
            "session_key": map[string]string{"type": "string", "description": "The session key to query"},
        },
    },
},
```

- [ ] **Step 4.4 — Add handler method**

Append to `internal/mcp/tools.go`:

```go
func (s *MCPServer) toolListSessionContexts(args json.RawMessage) (any, error) {
	var params struct {
		SessionKey string `json:"session_key"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.SessionKey == "" {
		return nil, fmt.Errorf("session_key is required")
	}
	agents, err := s.sessionStore.ListSessionContexts(context.Background(), params.SessionKey)
	if err != nil {
		return nil, fmt.Errorf("list session contexts: %w", err)
	}
	return agents, nil
}
```

- [ ] **Step 4.5 — Add case in `callTool` switch**

In `internal/mcp/tools.go`, add to the `switch name` block:

```go
case toolnames.ListSessionContexts:
    return s.toolListSessionContexts(args)
```

- [ ] **Step 4.6 — Update schema count assertion**

In `internal/mcp/tools_test.go`, change line ~349:

```go
// Before:
if len(schemas) != 18 {
    t.Errorf("expected 18 tool schemas, got %d", len(schemas))
}
// After (will become 20 after Task 5 adds another tool; update to 19 now, then 20 in Task 5):
if len(schemas) != 19 {
    t.Errorf("expected 19 tool schemas, got %d", len(schemas))
}
```

- [ ] **Step 4.7 — Run tests**

```bash
go test ./internal/mcp/... -run "TestCallTool_ListSessionContexts|TestToolSchemas_Count_AfterNewTools" -v
```

Expected: All PASS

- [ ] **Step 4.8 — Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_test.go
git commit -m "feat: add list_session_contexts MCP tool"
```

---

## Task 5: Add `clear_worker_session` tool

**Files:**
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/tools_test.go`

- [ ] **Step 5.1 — Write failing tests**

Append to `internal/mcp/tools_test.go`:

```go
// --- clear_worker_session ---

func TestCallTool_ClearWorkerSession_MissingSessionKey(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool("clear_worker_session", mustMarshal(t, map[string]any{
		"worker_id": "some-worker",
	}))
	if err == nil {
		t.Error("expected error for missing session_key")
	}
}

func TestCallTool_ClearWorkerSession_MissingWorkerID(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool("clear_worker_session", mustMarshal(t, map[string]any{
		"session_key": "sk",
	}))
	if err == nil {
		t.Error("expected error for missing worker_id")
	}
}

func TestCallTool_ClearWorkerSession_RefusesBee(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool("clear_worker_session", mustMarshal(t, map[string]any{
		"session_key": "sk",
		"worker_id":   "bee",
	}))
	if err == nil {
		t.Error("expected error when worker_id is bee")
	}
}

func TestCallTool_ClearWorkerSession_Idempotent(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	// No session row exists; should succeed without error.
	result, err := s.CallTool("clear_worker_session", mustMarshal(t, map[string]any{
		"session_key": "sk",
		"worker_id":   "nonexistent-worker-id",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["cleared"] != true {
		t.Errorf("expected cleared=true, got %v", m["cleared"])
	}
}

func TestCallTool_ClearWorkerSession_ClearsOnlyTargetWorker(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ctx := context.Background()

	// Create two workers
	workerResult1, _ := s.CallTool("create_worker", mustMarshal(t, map[string]any{"name": "W1"}))
	workerResult2, _ := s.CallTool("create_worker", mustMarshal(t, map[string]any{"name": "W2"}))
	w1 := workerResult1.(model.Worker)
	w2 := workerResult2.(model.Worker)

	// Seed session contexts for both workers
	ss := store.NewSessionStore(db)
	ss.UpsertSessionContext(ctx, "sk", w1.ID, "sid-w1") //nolint
	ss.UpsertSessionContext(ctx, "sk", w2.ID, "sid-w2") //nolint

	// Clear only w1
	result, err := s.CallTool("clear_worker_session", mustMarshal(t, map[string]any{
		"session_key": "sk",
		"worker_id":   w1.ID,
	}))
	if err != nil {
		t.Fatalf("clear_worker_session: %v", err)
	}
	m := result.(map[string]any)
	if m["cleared"] != true {
		t.Errorf("expected cleared=true, got %v", m["cleared"])
	}
	if m["worker_name"] != "W1" {
		t.Errorf("expected worker_name=W1, got %v", m["worker_name"])
	}

	// w1 context should be gone; w2 should remain
	w1sid, _ := ss.GetSessionContext(ctx, "sk", w1.ID)
	w2sid, _ := ss.GetSessionContext(ctx, "sk", w2.ID)
	if w1sid != "" {
		t.Errorf("expected w1 context cleared, got %q", w1sid)
	}
	if w2sid != "sid-w2" {
		t.Errorf("expected w2 context intact, got %q", w2sid)
	}
}
```

- [ ] **Step 5.2 — Run tests to verify they fail**

```bash
go test ./internal/mcp/... -run "TestCallTool_ClearWorkerSession" -v
```

Expected: FAIL — `unknown tool: clear_worker_session`

- [ ] **Step 5.3 — Add schema entry**

In `internal/mcp/tools.go`, append another new entry to `toolSchemas()`:

```go
{
    Name:        toolnames.ClearWorkerSession,
    Description: "Reset one worker's Claude session context within a session, without affecting other workers or bee. Does not cancel tasks.",
    InputSchema: map[string]any{
        "type":     "object",
        "required": []string{"session_key", "worker_id"},
        "properties": map[string]any{
            "session_key": map[string]string{"type": "string", "description": "The session key"},
            "worker_id":   map[string]string{"type": "string", "description": "Worker ID whose session context to delete"},
        },
    },
},
```

- [ ] **Step 5.4 — Add handler method**

Append to `internal/mcp/tools.go`:

```go
func (s *MCPServer) toolClearWorkerSession(args json.RawMessage) (any, error) {
	var params struct {
		SessionKey string `json:"session_key"`
		WorkerID   string `json:"worker_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.SessionKey == "" {
		return nil, fmt.Errorf("session_key is required")
	}
	if params.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}
	if params.WorkerID == store.BeeAgentID {
		return nil, fmt.Errorf("cannot clear bee session context with this tool, use clear_session instead")
	}

	ctx := context.Background()

	// Resolve worker name regardless of whether a session row exists.
	workerName := "(deleted)"
	if w, err := s.workerStore.GetByID(params.WorkerID); err == nil {
		workerName = w.Name
	}

	if err := s.sessionStore.DeleteWorkerSessionContext(ctx, params.SessionKey, params.WorkerID); err != nil {
		return nil, fmt.Errorf("delete worker session context: %w", err)
	}

	return map[string]any{
		"cleared":     true,
		"worker_id":   params.WorkerID,
		"worker_name": workerName,
	}, nil
}
```

- [ ] **Step 5.5 — Add case in `callTool` switch**

```go
case toolnames.ClearWorkerSession:
    return s.toolClearWorkerSession(args)
```

- [ ] **Step 5.6 — Update schema count assertion from 19 to 20**

```go
if len(schemas) != 20 {
    t.Errorf("expected 20 tool schemas, got %d", len(schemas))
}
```

- [ ] **Step 5.7 — Run tests**

```bash
go test ./internal/mcp/... -run "TestCallTool_ClearWorkerSession|TestToolSchemas_Count_AfterNewTools" -v
```

Expected: All PASS

- [ ] **Step 5.8 — Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_test.go
git commit -m "feat: add clear_worker_session MCP tool"
```

---

## Task 6: Modify `clear_session` — add `force` parameter + confirmation logic

**Files:**
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/tools_test.go`

- [ ] **Step 6.1 — Write failing tests for confirmation behavior**

Append to `internal/mcp/tools_test.go`:

```go
// --- clear_session confirmation ---

func TestCallTool_ClearSession_RequiresConfirmation_TwoWorkers(t *testing.T) {
	s, db, _, clearer := setupMCPServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-conf1", "session-C", "feishu", "hi", `{}`, "", 0) //nolint

	// Create two workers and seed session contexts for both.
	workerResult1, _ := s.CallTool("create_worker", mustMarshal(t, map[string]any{"name": "W1"}))
	workerResult2, _ := s.CallTool("create_worker", mustMarshal(t, map[string]any{"name": "W2"}))
	w1 := workerResult1.(model.Worker)
	w2 := workerResult2.(model.Worker)

	ss := store.NewSessionStore(db)
	ss.UpsertSessionContext(ctx, "session-C", w1.ID, "sid-w1") //nolint
	ss.UpsertSessionContext(ctx, "session-C", w2.ID, "sid-w2") //nolint

	// Call without force — should get confirmation request, NOT clear.
	result, err := s.CallTool("clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-C",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["requires_confirmation"] != true {
		t.Errorf("expected requires_confirmation=true, got %v", m["requires_confirmation"])
	}
	workerCount, _ := m["worker_count"].(int)
	if workerCount != 2 {
		t.Errorf("expected worker_count=2, got %v", m["worker_count"])
	}
	linkedWorkers, _ := m["linked_workers"].([]map[string]string)
	if len(linkedWorkers) != 2 {
		t.Errorf("expected 2 linked_workers, got %v", m["linked_workers"])
	}

	// ClearSession must NOT have been called.
	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) != 0 {
		t.Errorf("ClearSession must not be called on confirmation prompt, got %v", clearer.cleared)
	}
}

func TestCallTool_ClearSession_ForceTrue_SkipsConfirmation(t *testing.T) {
	s, db, _, clearer := setupMCPServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-force1", "session-F", "feishu", "hi", `{}`, "", 0) //nolint

	workerResult1, _ := s.CallTool("create_worker", mustMarshal(t, map[string]any{"name": "W1"}))
	workerResult2, _ := s.CallTool("create_worker", mustMarshal(t, map[string]any{"name": "W2"}))
	w1 := workerResult1.(model.Worker)
	w2 := workerResult2.(model.Worker)

	ss := store.NewSessionStore(db)
	ss.UpsertSessionContext(ctx, "session-F", w1.ID, "sid-w1") //nolint
	ss.UpsertSessionContext(ctx, "session-F", w2.ID, "sid-w2") //nolint

	result, err := s.CallTool("clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-F",
		"force":       true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["cleared"] != true {
		t.Errorf("expected cleared=true, got %v", m["cleared"])
	}

	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) != 1 || clearer.cleared[0] != "session-F" {
		t.Errorf("expected ClearSession(session-F), got %v", clearer.cleared)
	}
}

func TestCallTool_ClearSession_OneWorker_NoConfirmation(t *testing.T) {
	s, db, _, clearer := setupMCPServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-one1", "session-O", "feishu", "hi", `{}`, "", 0) //nolint

	workerResult, _ := s.CallTool("create_worker", mustMarshal(t, map[string]any{"name": "W"}))
	w := workerResult.(model.Worker)

	ss := store.NewSessionStore(db)
	ss.UpsertSessionContext(ctx, "session-O", w.ID, "sid-w") //nolint

	// Only 1 worker — should clear without confirmation.
	result, err := s.CallTool("clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-O",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["cleared"] != true {
		t.Errorf("expected cleared=true, got %v", m["cleared"])
	}

	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) != 1 {
		t.Errorf("expected ClearSession called once, got %v", clearer.cleared)
	}
}
```

- [ ] **Step 6.2 — Run tests to verify they fail**

```bash
go test ./internal/mcp/... -run "TestCallTool_ClearSession_RequiresConfirmation|TestCallTool_ClearSession_ForceTrue|TestCallTool_ClearSession_OneWorker" -v
```

Expected: `TestCallTool_ClearSession_RequiresConfirmation_TwoWorkers` FAIL — no confirmation returned.

- [ ] **Step 6.3 — Update `clear_session` schema to advertise `force` field**

In `toolSchemas()`, find the `clear_session` entry and update its `InputSchema`:

```go
{
    Name:        toolnames.ClearSession,
    Description: "Cancel all active tasks (terminating running worker processes), clear dispatcher queues, and reset all session contexts for the given session. Use this to fully reset a conversation session.",
    InputSchema: map[string]any{
        "type":     "object",
        "required": []string{"session_key"},
        "properties": map[string]any{
            "session_key": map[string]string{"type": "string", "description": "The session key to clear"},
            "force":       map[string]any{"type": "boolean", "description": "Skip confirmation when multiple workers are linked; default false", "default": false},
        },
    },
},
```

- [ ] **Step 6.4 — Modify `toolClearSession` handler**

Replace the existing `toolClearSession` method body. Add `Force bool` to the params struct and insert confirmation logic before the existing steps:

```go
func (s *MCPServer) toolClearSession(args json.RawMessage) (any, error) {
	var params struct {
		SessionKey string `json:"session_key"`
		Force      bool   `json:"force"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.SessionKey == "" {
		return nil, fmt.Errorf("session_key is required")
	}

	ctx := context.Background()

	// Two-step confirmation: if more than one worker has a session context and
	// force is not set, return a confirmation prompt without clearing anything.
	if !params.Force {
		agents, err := s.sessionStore.ListSessionContexts(ctx, params.SessionKey)
		if err != nil {
			return nil, fmt.Errorf("list session contexts: %w", err)
		}
		var workers []map[string]string
		for _, a := range agents {
			if a.AgentType == "worker" {
				workers = append(workers, map[string]string{
					"worker_id": a.AgentID,
					"name":      a.Name,
				})
			}
		}
		if len(workers) > 1 {
			return map[string]any{
				"requires_confirmation": true,
				"worker_count":          len(workers),
				"linked_workers":        workers,
				"message":               fmt.Sprintf("此会话链接了 %d 位员工，清空将重置所有员工和 bee 的对话上下文。请确认后以 force=true 重新调用。", len(workers)),
			}, nil
		}
	}

	// --- existing logic below (unchanged) ---

	// Step 1: Collect running tasks with execution IDs (before cancelling)
	runningTasks, err := s.taskStore.ListBySessionKey(ctx, params.SessionKey, "running", "")
	if err != nil {
		return nil, fmt.Errorf("list running tasks: %w", err)
	}

	// Step 2: Stop running worker processes
	for _, t := range runningTasks {
		if t.ExecutionID != "" {
			if err := s.execStopper.StopExecution(t.ExecutionID); err != nil {
				log.Error("stop execution", zap.String("op", "clear_session"), zap.String("executionID", t.ExecutionID), zap.Error(err))
			}
		}
	}

	// Step 3: Cancel all pending/running tasks in DB
	cancelled, err := s.taskStore.CancelBySessionKey(ctx, params.SessionKey)
	if err != nil {
		return nil, fmt.Errorf("cancel tasks: %w", err)
	}

	// Step 4: Clear dispatcher queues + session contexts
	if s.sessionClearer != nil {
		s.sessionClearer.ClearSession(params.SessionKey)
	}

	return map[string]any{
		"cancelled_tasks": cancelled,
		"cleared":         true,
	}, nil
}
```

- [ ] **Step 6.5 — Run all `clear_session` tests**

```bash
go test ./internal/mcp/... -run "TestCallTool_ClearSession" -v
```

Expected: All PASS (including old tests for no-active-tasks, cancels-and-stops, missing-key)

- [ ] **Step 6.6 — Run full test suite**

```bash
go test ./... 2>&1 | tail -30
```

Expected: All PASS

- [ ] **Step 6.7 — Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_test.go
git commit -m "feat: add force confirmation to clear_session MCP tool"
```

---

## Final Verification

- [ ] **Run full test suite one more time**

```bash
go test ./... -count=1
```

Expected: All PASS, no compilation errors.

- [ ] **Verify all three new tools appear in schema list**

```bash
go test ./internal/mcp/... -run TestToolSchemas_IncludesNewTools -v
```

Also verify by inspection:
```bash
grep -A2 'Name:.*session' internal/mcp/tools.go
```

Expected: `list_session_contexts` and `clear_worker_session` entries visible.
