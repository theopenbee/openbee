# Remove MCP SSE Streaming Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the SSE streaming layer from the MCP server, leaving only the `/mcp/bee/call` direct-call endpoint that `openbee ctl` uses.

**Architecture:** The `MCPServer` struct currently manages both SSE sessions (for MCP protocol clients) and the simple `HandleCall` HTTP handler. We delete all SSE/JSON-RPC session infrastructure while keeping `HandleCall`, `beeCallTool`, and all store dependencies. No new code is written — this is purely a deletion.

**Tech Stack:** Go, Gin, `internal/ai/mcp`, `internal/api`, `internal/app`, `internal/infra/config`

---

## File Map

| File | Change |
|------|--------|
| `internal/ai/mcp/tools.go` | Delete worker tool functions (lines 33–34, 317–335, 411–416) |
| `internal/ai/mcp/tools_test.go` | Delete worker server helper and 3 worker-specific tests |
| `internal/ai/mcp/server.go` | Delete SSE methods, JSON-RPC types, NewWorkerServer, dead fields |
| `internal/api/router.go` | Remove WorkerMCPServer field, SSE routes, gzip exclusions |
| `internal/app/app.go` | Remove workerMCPSrv creation and passing |
| `internal/infra/config/config.go` | Remove MCPWorkerBasePath constant |

---

## Task 1: Delete worker tool code from tools.go

**Files:**
- Modify: `internal/ai/mcp/tools.go`

- [ ] **Step 1: Verify tests pass before any changes**

```bash
go test ./internal/ai/mcp/... -count=1
```
Expected: all tests PASS

- [ ] **Step 2: Delete the `WorkerToolSchemas` exported function (line 33–34)**

Remove these two lines from `internal/ai/mcp/tools.go`:
```go
// WorkerToolSchemas returns the JSON Schema definitions for Worker MCP tools.
func WorkerToolSchemas() []toolSchema { return workerToolSchemas() }
```

- [ ] **Step 3: Delete `workerToolNames` and `workerToolSchemas` (lines 317–335)**

Remove these lines from `internal/ai/mcp/tools.go`:
```go
// workerToolNames is the allowlist of tools exposed to workers.
var workerToolNames = map[string]bool{
	utils.SendMessage:      true,
	utils.SaveMemory:       true,
	utils.GetMemory:        true,
	utils.DeleteMemory:     true,
}

func workerToolSchemas() []toolSchema {
	all := beeToolSchemas()
	out := make([]toolSchema, 0, len(workerToolNames))
	for _, s := range all {
		if workerToolNames[s.Name] {
			out = append(out, s)
		}
	}
	return out
}
```

- [ ] **Step 4: Delete `workerCallTool` method (lines 411–416)**

Remove these lines from `internal/ai/mcp/tools.go`:
```go
// workerCallTool delegates to beeCallTool after checking the worker allowlist.
func (s *MCPServer) workerCallTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	if !workerToolNames[name] {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return s.beeCallTool(ctx, name, args)
}
```

- [ ] **Step 5: Verify it compiles (tests will fail — that's expected until Task 2)**

```bash
go build ./internal/ai/mcp/...
```
Expected: compile error referencing `workerCallTool` and `NewWorkerServer` — that's fine; we'll fix them in Task 2.

---

## Task 2: Delete worker tests from tools_test.go

**Files:**
- Modify: `internal/ai/mcp/tools_test.go`

- [ ] **Step 1: Delete `setupWorkerMCPServer` helper (lines 935–949)**

Remove these lines from `internal/ai/mcp/tools_test.go`:
```go
func setupWorkerMCPServer(t *testing.T) *mcp.MCPServer {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ts := store.NewTaskStore(db)
	ms := store.NewMessageStore(db)
	senders := make(map[string]platform.PlatformSenderAdapter)
	memStore := store.NewMemoryStore(db)
	ws := store.NewWorkerStore(db)
	return mcp.NewWorkerServer(ts, ms, senders, memStore, ws)
}
```

- [ ] **Step 2: Delete `TestWorkerToolSchemasCount` test (lines 958–963)**

Remove:
```go
func TestWorkerToolSchemasCount(t *testing.T) {
	schemas := mcp.WorkerToolSchemas()
	if len(schemas) != 4 {
		t.Errorf("worker tool schemas: want 4 got %d", len(schemas))
	}
}
```

- [ ] **Step 3: Delete `TestWorkerCannotCallBeeTools` test (lines 965–992)**

Remove:
```go
func TestWorkerCannotCallBeeTools(t *testing.T) {
	s := setupWorkerMCPServer(t)
	beeOnlyTools := []string{
		utils.ListWorkers,
		utils.GetWorker,
		utils.CreateWorker,
		utils.UpdateWorker,
		utils.DeleteWorker,
		utils.CreateTask,
		utils.ListTasks,
		utils.CancelTask,
		utils.ClearSession,
		utils.GetWorkerStatus,
		utils.GetSystemOverview,
		utils.ListBeeExecutions,
		utils.ListSessionContexts,
		utils.ClearWorkerSession,
	}
	for _, tool := range beeOnlyTools {
		_, err := s.CallTool(context.Background(), tool, mustMarshal(t, map[string]any{}))
		if err == nil {
			t.Errorf("worker should not be able to call %s", tool)
		}
		if !strings.Contains(err.Error(), "unknown tool") {
			t.Errorf("CallTool(%s): want 'unknown tool' error, got: %v", tool, err)
		}
	}
}
```

- [ ] **Step 4: Delete `TestWorkerCanCallAllowedTools` test**

Find and remove `TestWorkerCanCallAllowedTools` — the function starts at the line after `TestWorkerCannotCallBeeTools` and covers the 4 allowed worker tools (`SendMessage`, `SaveMemory`, `GetMemory`, `DeleteMemory`). Delete the entire function body.

---

## Task 3: Simplify server.go — remove SSE and JSON-RPC code

**Files:**
- Modify: `internal/ai/mcp/server.go`

- [ ] **Step 1: Delete JSON-RPC types and helpers (lines 32–61)**

Remove from `internal/ai/mcp/server.go`:
```go
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func errResponse(id any, code int, msg string) rpcResponse {
	return rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: msg},
	}
}

func okResponse(id any, result any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}
```

- [ ] **Step 2: Remove dead fields from MCPServer struct**

In `internal/ai/mcp/server.go`, update the `MCPServer` struct by removing these fields:
```go
schemasFn  func() []toolSchema
basePath   string // SSE endpoint URL prefix, e.g. "/mcp/bee"
mu              sync.Mutex
sessions        map[string]chan rpcResponse // session_id -> response channel
```

The struct after change should look like:
```go
// MCPServer dispatches JSON tool calls from the /mcp/bee/call endpoint.
type MCPServer struct {
	callToolFn func(ctx context.Context, name string, args json.RawMessage) (any, error)

	workerStore    *store.WorkerStore
	manager        *worker.Manager
	taskStore      *store.TaskStore
	messageStore   *store.MessageStore
	senders        map[string]platform.PlatformSenderAdapter
	execStopper    ExecutionStopper
	sessionClearer SessionClearer
	executionStore *store.ExecutionStore
	memoryStore     *store.MemoryStore
	sessionStore    *store.SessionStore
	departmentStore *store.DepartmentStore

	workerNameCache sync.Map // workerID -> display name; lazily populated
}
```

- [ ] **Step 3: Update NewBeeServer — remove basePath and sessions init**

In `internal/ai/mcp/server.go`, replace the body of `NewBeeServer` to remove the two deleted fields:

Old:
```go
	s := &MCPServer{
		basePath:        config.MCPBeeBasePath,
		workerStore:     ws,
		manager:         mgr,
		taskStore:       ts,
		messageStore:    ms,
		senders:         senders,
		execStopper:     execStopper,
		sessionClearer:  sessionClearer,
		executionStore:  es,
		memoryStore:     memStore,
		sessionStore:    sessionStore,
		departmentStore: ds,
		sessions:        make(map[string]chan rpcResponse),
	}
	s.schemasFn = beeToolSchemas
	s.callToolFn = s.beeCallTool
```

New:
```go
	s := &MCPServer{
		workerStore:     ws,
		manager:         mgr,
		taskStore:       ts,
		messageStore:    ms,
		senders:         senders,
		execStopper:     execStopper,
		sessionClearer:  sessionClearer,
		executionStore:  es,
		memoryStore:     memStore,
		sessionStore:    sessionStore,
		departmentStore: ds,
	}
	s.callToolFn = s.beeCallTool
```

- [ ] **Step 4: Delete NewWorkerServer function (lines 130–150)**

Remove from `internal/ai/mcp/server.go`:
```go
// NewWorkerServer creates a Worker MCP Server with a restricted tool set.
func NewWorkerServer(
	ts *store.TaskStore,
	ms *store.MessageStore,
	senders map[string]platform.PlatformSenderAdapter,
	memStore *store.MemoryStore,
	ws *store.WorkerStore,
) *MCPServer {
	s := &MCPServer{
		basePath:     config.MCPWorkerBasePath,
		taskStore:    ts,
		messageStore: ms,
		senders:      senders,
		memoryStore:  memStore,
		workerStore:  ws,
		sessions:     make(map[string]chan rpcResponse),
	}
	s.schemasFn = workerToolSchemas
	s.callToolFn = s.workerCallTool
	return s
}
```

- [ ] **Step 5: Delete HandleSSE method (lines 152–212)**

Remove the entire `HandleSSE` method from `internal/ai/mcp/server.go` (the method starting with `// HandleSSE establishes the SSE connection...`).

- [ ] **Step 6: Delete HandleMessages method (lines 214–242)**

Remove the entire `HandleMessages` method from `internal/ai/mcp/server.go`.

- [ ] **Step 7: Delete dispatch and handleToolCall methods (lines 248–323)**

Remove both the `dispatch` and `handleToolCall` methods from `internal/ai/mcp/server.go`.

- [ ] **Step 8: Fix imports — remove unused imports**

Update the import block in `internal/ai/mcp/server.go`. Remove:
- `"net/url"`
- `"sync"` — **keep** this import because `workerNameCache sync.Map` in the struct still uses it
- `"time"`
- `"fmt"` — **keep** this if `HandleCall` or other remaining code uses it (it doesn't — remove it)
- `"github.com/google/uuid"` — only used in HandleSSE, remove it
- `"github.com/theopenbee/openbee/internal/infra/config"` — only used in NewBeeServer/NewWorkerServer for path constants, remove it

After cleanup, the import block should be:
```go
import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/domain/worker"
)
```

- [ ] **Step 9: Verify tests pass**

```bash
go test ./internal/ai/mcp/... -count=1
```
Expected: all tests PASS

- [ ] **Step 10: Commit**

```bash
git add internal/ai/mcp/server.go internal/ai/mcp/tools.go internal/ai/mcp/tools_test.go
git commit -m "refactor: remove MCP SSE streaming layer and worker server"
```

---

## Task 4: Update router.go — remove SSE routes and WorkerMCPServer

**Files:**
- Modify: `internal/api/router.go`

- [ ] **Step 1: Remove WorkerMCPServer from ServerParams**

In `internal/api/router.go`, delete this line from the `ServerParams` struct:
```go
WorkerMCPServer  *mcp.MCPServer
```

- [ ] **Step 2: Remove SSE gzip exclusions**

In `NewServer()` in `internal/api/router.go`, update the gzip exclusion list:

Old:
```go
	router.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPathsRegexs([]string{
		"/api/local/stream",
		"/mcp/.*/sse",
		"/mcp/.*/messages",
	})))
```

New:
```go
	router.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPathsRegexs([]string{
		"/api/local/stream",
	})))
```

- [ ] **Step 3: Rewrite registerMCPRoutes to remove SSE routes and workerGroup**

Old `registerMCPRoutes`:
```go
func (s *Server) registerMCPRoutes() {
	beeGroup := s.router.Group(config.MCPBeeBasePath)
	beeGroup.Use(mcp.JWTAuthMiddleware(s.TokenSecret), mcp.RequireBee())
	beeGroup.GET("/sse", s.BeeMCPServer.HandleSSE)
	beeGroup.POST("/messages", s.BeeMCPServer.HandleMessages)

	s.router.POST(config.MCPBeeBasePath+"/call",
		mcp.JWTAuthMiddleware(s.TokenSecret),
		mcp.RequireBeeOrWorker(),
		s.BeeMCPServer.HandleCall,
	)

	workerGroup := s.router.Group(config.MCPWorkerBasePath)
	workerGroup.Use(mcp.JWTAuthMiddleware(s.TokenSecret), mcp.RequireWorker())
	workerGroup.GET("/sse", s.WorkerMCPServer.HandleSSE)
	workerGroup.POST("/messages", s.WorkerMCPServer.HandleMessages)
}
```

New `registerMCPRoutes`:
```go
func (s *Server) registerMCPRoutes() {
	s.router.POST(config.MCPBeeBasePath+"/call",
		mcp.JWTAuthMiddleware(s.TokenSecret),
		mcp.RequireBeeOrWorker(),
		s.BeeMCPServer.HandleCall,
	)
}
```

- [ ] **Step 4: Remove unused RequireWorker from the mcp import if needed**

Check if `mcp.RequireWorker` is still referenced anywhere in `router.go`. Since `workerGroup` is gone, it will no longer be referenced. The `mcp` import itself is still needed for `JWTAuthMiddleware`, `RequireBeeOrWorker`, and `*mcp.MCPServer` — so keep the import.

Also check if `config.MCPWorkerBasePath` is referenced in `router.go` — it was used in the deleted `workerGroup`. Confirm it is now gone.

- [ ] **Step 5: Build to verify**

```bash
go build ./internal/api/...
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/api/router.go
git commit -m "refactor: remove SSE routes from MCP router"
```

---

## Task 5: Update app.go — remove workerMCPSrv

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Delete workerMCPSrv creation line**

In `internal/app/app.go`, delete this line (currently line 116):
```go
workerMCPSrv := mcp.NewWorkerServer(s.taskStore, s.msgStore, sendersByPlatform, s.memoryStore, s.workerStore)
```

- [ ] **Step 2: Remove workerMCPSrv from buildAPIServer call**

Old (line 154):
```go
	srv, err := buildAPIServer(cfg.Server, cfg.Bee.MCP, s, mgr, beeMCPSrv, workerMCPSrv, localChatHandler, cfg.Language)
```

New:
```go
	srv, err := buildAPIServer(cfg.Server, cfg.Bee.MCP, s, mgr, beeMCPSrv, localChatHandler, cfg.Language)
```

- [ ] **Step 3: Update buildAPIServer signature and body**

Old signature (line 240):
```go
func buildAPIServer(serverCfg config.ServerConfig, mcpCfg config.MCPConfig, s appStores, mgr *worker.Manager, beeMCPSrv *mcp.MCPServer, workerMCPSrv *mcp.MCPServer, localChat *api.LocalChatHandler, language string) (*api.Server, error) {
```

New signature:
```go
func buildAPIServer(serverCfg config.ServerConfig, mcpCfg config.MCPConfig, s appStores, mgr *worker.Manager, beeMCPSrv *mcp.MCPServer, localChat *api.LocalChatHandler, language string) (*api.Server, error) {
```

Also remove `WorkerMCPServer: workerMCPSrv,` from the `api.NewServer(api.ServerParams{...})` call inside `buildAPIServer`.

- [ ] **Step 4: Build to verify**

```bash
go build ./...
```
Expected: PASS

- [ ] **Step 5: Run full test suite**

```bash
go test ./... -count=1
```
Expected: all tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/app/app.go
git commit -m "refactor: remove workerMCPSrv from app wiring"
```

---

## Task 6: Remove MCPWorkerBasePath constant from config

**Files:**
- Modify: `internal/infra/config/config.go`

- [ ] **Step 1: Delete MCPWorkerBasePath constant**

In `internal/infra/config/config.go`, remove this line from the constants block:
```go
MCPWorkerBasePath = "/mcp/worker"
```

The block should go from:
```go
const (
	MCPBeeBasePath    = "/mcp/bee"
	MCPWorkerBasePath = "/mcp/worker"
)
```

To:
```go
const (
	MCPBeeBasePath = "/mcp/bee"
)
```

- [ ] **Step 2: Build to verify**

```bash
go build ./...
```
Expected: PASS

- [ ] **Step 3: Run full test suite**

```bash
go test ./... -count=1
```
Expected: all tests PASS

- [ ] **Step 4: Smoke test — verify openbee ctl still works**

```bash
./openbee ctl worker list
```
Expected: JSON response listing workers (or empty list `[]`), no error

- [ ] **Step 5: Final commit**

```bash
git add internal/infra/config/config.go
git commit -m "refactor: remove MCPWorkerBasePath constant"
```

---

## Spec Coverage Check

| Spec requirement | Task |
|-----------------|------|
| Delete workerToolNames, workerToolSchemas, WorkerToolSchemas | Task 1 |
| Delete workerCallTool | Task 1 |
| Delete NewWorkerServer | Task 3, Step 4 |
| Delete HandleSSE | Task 3, Step 5 |
| Delete HandleMessages | Task 3, Step 6 |
| Delete dispatch + handleToolCall | Task 3, Step 7 |
| Delete rpc* types + errResponse/okResponse | Task 3, Step 1 |
| Remove basePath, sessions, mu, schemasFn fields | Task 3, Steps 2–3 |
| Remove WorkerMCPServer from router ServerParams | Task 4, Step 1 |
| Remove SSE gzip exclusions | Task 4, Step 2 |
| Remove SSE/messages routes + workerGroup | Task 4, Step 3 |
| Remove workerMCPSrv from app.go | Task 5 |
| Remove MCPWorkerBasePath constant | Task 6 |
| Verify ctl still works | Task 6, Step 4 |
