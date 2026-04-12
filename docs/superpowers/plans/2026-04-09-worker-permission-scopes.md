# Worker Permission Scopes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `permission_scopes` to workers so admins can grant read-only access to `openbee ctl` org-query commands (workers, departments, tasks).

**Architecture:** Scopes are stored in the `bee_workers` table and embedded in worker JWTs. The MCP tool dispatcher checks JWT scopes before allowing worker tokens to call the new read-only tools. The `openbee ctl` client already prefers `OPENBEE_API_KEY` (the worker token injected at runtime), so no ctl-client changes are needed beyond adding the `--scopes` flag to `create`/`update`.

**Tech Stack:** Go, SQLite (modernc), JWT (golang-jwt/jwt/v5), Cobra CLI, Gin

---

## File Map

| File | Action | Purpose |
|------|--------|---------|
| `internal/infra/auth/scopes.go` | **Create** | Scope constants + ToolScopeMap |
| `internal/infra/auth/token.go` | **Modify** | Add `Scopes []string` to MCPClaims; update GenerateWorkerToken signature |
| `internal/infra/auth/token_test.go` | **Modify** | Update worker token tests for new signature |
| `internal/infra/model/worker.go` | **Modify** | Add `PermissionScopes string` field |
| `internal/infra/store/db.go` | **Modify** | Add migration 29: ALTER TABLE bee_workers ADD COLUMN permission_scopes |
| `internal/infra/store/worker_store.go` | **Modify** | Include permission_scopes in workerColumns, scanWorker, Create, Update |
| `internal/domain/worker/manager.go` | **Modify** | Parse scopes from worker model before calling GenerateWorkerToken |
| `internal/mcp/server.go` | **Modify** | Add CtxScopesKey; propagate scopes from gin ctx to request ctx |
| `internal/mcp/auth.go` | **Modify** | Set scopes on gin context from JWT claims |
| `internal/mcp/auth_test.go` | **Modify** | Add test: scopes stored in context |
| `internal/mcp/tools.go` | **Modify** | Add checkWorkerScope helper; call at top of beeCallTool |
| `internal/mcp/tools_test.go` | **Modify** | Add scope enforcement tests |
| `cmd/openbee/ctl_worker.go` | **Modify** | Add --scopes flag to create and update commands |
| `internal/infra/skillinstall/skills/openbee-worker/SKILL.md` | **Modify** | Add read-only commands section |

---

## Task 1: Scope Constants

**Files:**
- Create: `internal/infra/auth/scopes.go`

- [ ] **Step 1: Create the scopes file**

```go
// internal/infra/auth/scopes.go
package auth

import "github.com/theopenbee/openbee/internal/infra/utils"

const (
	ScopeReadWorkers     = "read:workers"
	ScopeReadDepartments = "read:departments"
	ScopeReadTasks       = "read:tasks"
)

// ToolScopeMap maps tool names to the scope required for worker-token callers.
// Tools in this map require the listed scope when called with a worker token.
// Tools absent from this map follow existing access rules (unchanged behavior).
var ToolScopeMap = map[string]string{
	utils.ListWorkers:     ScopeReadWorkers,
	utils.GetWorker:       ScopeReadWorkers,
	utils.GetWorkerStatus: ScopeReadWorkers,
	utils.ListDepartments: ScopeReadDepartments,
	utils.GetDepartment:   ScopeReadDepartments,
	utils.ListTasks:       ScopeReadTasks,
}
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
go build ./internal/infra/auth/...
```

Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add internal/infra/auth/scopes.go
git commit -m "feat: add worker permission scope constants and tool scope map"
```

---

## Task 2: Extend MCPClaims and GenerateWorkerToken

**Files:**
- Modify: `internal/infra/auth/token.go`
- Modify: `internal/infra/auth/token_test.go`

- [ ] **Step 1: Write the failing test first**

In `internal/infra/auth/token_test.go`, add after the existing tests:

```go
func TestGenerateWorkerToken_WithScopes(t *testing.T) {
	scopes := []string{"read:workers", "read:tasks"}
	tok, err := auth.GenerateWorkerToken("test-secret", "worker-abc", scopes, time.Hour)
	if err != nil {
		t.Fatalf("GenerateWorkerToken: %v", err)
	}
	claims, err := auth.ValidateToken(tok, "test-secret")
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Type != auth.TokenTypeWorker {
		t.Errorf("Type: want %q got %q", auth.TokenTypeWorker, claims.Type)
	}
	if claims.WorkerID != "worker-abc" {
		t.Errorf("WorkerID: want worker-abc got %q", claims.WorkerID)
	}
	if len(claims.Scopes) != 2 || claims.Scopes[0] != "read:workers" || claims.Scopes[1] != "read:tasks" {
		t.Errorf("Scopes: want [read:workers read:tasks] got %v", claims.Scopes)
	}
}

func TestGenerateWorkerToken_NoScopes(t *testing.T) {
	tok, err := auth.GenerateWorkerToken("test-secret", "worker-abc", nil, time.Hour)
	if err != nil {
		t.Fatalf("GenerateWorkerToken: %v", err)
	}
	claims, err := auth.ValidateToken(tok, "test-secret")
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if len(claims.Scopes) != 0 {
		t.Errorf("Scopes: want empty got %v", claims.Scopes)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/infra/auth/... -run "TestGenerateWorkerToken_WithScopes|TestGenerateWorkerToken_NoScopes" -v
```

Expected: FAIL — compile error because `GenerateWorkerToken` signature doesn't accept `[]string` yet.

- [ ] **Step 3: Update MCPClaims and GenerateWorkerToken**

In `internal/infra/auth/token.go`:

```go
// MCPClaims are the JWT claims embedded in every MCP token.
type MCPClaims struct {
	Type     string   `json:"type"`
	WorkerID string   `json:"worker_id,omitempty"`
	Scopes   []string `json:"scopes,omitempty"`
	jwt.RegisteredClaims
}

// GenerateWorkerToken creates a signed JWT for a specific Worker with optional scopes.
func GenerateWorkerToken(secret, workerID string, scopes []string, ttl time.Duration) (string, error) {
	return signToken(MCPClaims{
		Type:     TokenTypeWorker,
		WorkerID: workerID,
		Scopes:   scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
	}, secret)
}
```

- [ ] **Step 4: Fix the existing test that uses old signature**

In `internal/infra/auth/token_test.go`, update `TestGenerateWorkerToken_ValidAndParseable`:

```go
func TestGenerateWorkerToken_ValidAndParseable(t *testing.T) {
	tok, err := auth.GenerateWorkerToken("test-secret", "worker-abc", nil, time.Hour)
	if err != nil {
		t.Fatalf("GenerateWorkerToken: %v", err)
	}
	claims, err := auth.ValidateToken(tok, "test-secret")
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Type != auth.TokenTypeWorker {
		t.Errorf("Type: want %q got %q", auth.TokenTypeWorker, claims.Type)
	}
	if claims.WorkerID != "worker-abc" {
		t.Errorf("WorkerID: want worker-abc got %q", claims.WorkerID)
	}
}
```

- [ ] **Step 5: Run all auth tests**

```bash
go test ./internal/infra/auth/... -v
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/infra/auth/token.go internal/infra/auth/token_test.go
git commit -m "feat: add Scopes field to MCPClaims and update GenerateWorkerToken signature"
```

---

## Task 3: Worker Model and Database Migration

**Files:**
- Modify: `internal/infra/model/worker.go`
- Modify: `internal/infra/store/db.go`
- Modify: `internal/infra/store/worker_store.go`

- [ ] **Step 1: Add PermissionScopes to Worker model**

In `internal/infra/model/worker.go`, add the field:

```go
type Worker struct {
	ID               string       `json:"id" db:"id"`
	Name             string       `json:"name" db:"name"`
	Description      string       `json:"description" db:"description"`
	Memory           string       `json:"memory" db:"memory"`
	WorkDir          string       `json:"work_dir" db:"work_dir"`
	Status           WorkerStatus `json:"status" db:"status"`
	PermissionScopes string       `json:"permission_scopes" db:"permission_scopes"`
	CreatedAt        int64        `json:"created_at" db:"created_at"`
	UpdatedAt        int64        `json:"updated_at" db:"updated_at"`
}
```

- [ ] **Step 2: Add migration 29 to db.go**

In `internal/infra/store/db.go`, add after the last migration entry (version 28):

```go
{
    version: 29,
    name:    "add_permission_scopes_to_workers",
    sql:     `ALTER TABLE bee_workers ADD COLUMN permission_scopes TEXT NOT NULL DEFAULT ''`,
},
```

- [ ] **Step 3: Update worker_store.go to include permission_scopes**

In `internal/infra/store/worker_store.go`, update `workerColumns` and all affected functions:

```go
const workerColumns = `id, name, description, memory, work_dir, status, permission_scopes, created_at, updated_at`

func scanWorker(scanner interface{ Scan(...any) error }) (model.Worker, error) {
	var w model.Worker
	err := scanner.Scan(
		&w.ID, &w.Name, &w.Description, &w.Memory,
		&w.WorkDir, &w.Status, &w.PermissionScopes, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return model.Worker{}, err
	}
	return w, nil
}
```

Update `Create`:

```go
func (s *WorkerStore) Create(w model.Worker) (model.Worker, error) {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	w.Status = model.WorkerStatusIdle
	w.CreatedAt = time.Now().UnixMilli()
	w.UpdatedAt = w.CreatedAt

	_, err := s.db.Exec(
		`INSERT INTO bee_workers (id, name, description, memory, work_dir, status, permission_scopes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.Description, w.Memory, w.WorkDir,
		w.Status, w.PermissionScopes, w.CreatedAt, w.UpdatedAt,
	)
	if err != nil {
		return model.Worker{}, fmt.Errorf("insert worker: %w", err)
	}
	return w, nil
}
```

Update `Update`:

```go
func (s *WorkerStore) Update(w model.Worker) (model.Worker, error) {
	w.UpdatedAt = time.Now().UnixMilli()
	_, err := s.db.Exec(
		`UPDATE bee_workers SET name=?, description=?, memory=?, work_dir=?, status=?, permission_scopes=?, updated_at=?
		 WHERE id=?`,
		w.Name, w.Description, w.Memory, w.WorkDir,
		w.Status, w.PermissionScopes, w.UpdatedAt, w.ID,
	)
	if err != nil {
		return model.Worker{}, fmt.Errorf("update worker: %w", err)
	}
	return w, nil
}
```

Also update `GetByDepartmentID` — it uses an inline column list instead of `workerColumns`. Update it to include `permission_scopes`:

```go
func (s *WorkerStore) GetByDepartmentID(deptID string) ([]model.Worker, error) {
	rows, err := s.db.Query(
		`SELECT w.id, w.name, w.description, w.memory, w.work_dir, w.status, w.permission_scopes, w.created_at, w.updated_at
		 FROM bee_workers w
		 INNER JOIN bee_worker_departments wd ON w.id = wd.worker_id
		 WHERE wd.department_id = ?
		 ORDER BY w.created_at DESC`,
		deptID,
	)
	if err != nil {
		return nil, fmt.Errorf("get workers by department: %w", err)
	}
	defer rows.Close()
	return scanWorkers(rows)
}
```

- [ ] **Step 4: Build and run existing store/worker tests**

```bash
go build ./...
go test ./internal/infra/store/... -v
```

Expected: all PASS (migration runs automatically in tests via `store.InitDB`)

- [ ] **Step 5: Commit**

```bash
git add internal/infra/model/worker.go internal/infra/store/db.go internal/infra/store/worker_store.go
git commit -m "feat: add permission_scopes field to worker model and store"
```

---

## Task 4: Worker Manager — Pass Scopes to Token

**Files:**
- Modify: `internal/domain/worker/manager.go`

- [ ] **Step 1: Update GenerateWorkerToken call in launchRuntime**

In `internal/domain/worker/manager.go`, find the `launchRuntime` function. Replace the token generation lines:

```go
// Parse permission scopes from comma-separated string
var scopes []string
if worker.PermissionScopes != "" {
    for _, s := range strings.Split(worker.PermissionScopes, ",") {
        s = strings.TrimSpace(s)
        if s != "" {
            scopes = append(scopes, s)
        }
    }
}

token, err := auth.GenerateWorkerToken(m.beeCfg.MCP.TokenSecret, worker.ID, scopes, m.beeCfg.MCP.TokenTTL)
if err != nil {
    return fmt.Errorf("generate worker token: %w", err)
}
```

Add `"strings"` to the import block if not already present.

- [ ] **Step 2: Build to verify no compile errors**

```bash
go build ./internal/domain/worker/...
```

Expected: no output

- [ ] **Step 3: Commit**

```bash
git add internal/domain/worker/manager.go
git commit -m "feat: embed permission scopes in worker JWT during token generation"
```

---

## Task 5: MCP Auth — Propagate Scopes to Request Context

**Files:**
- Modify: `internal/mcp/auth.go`
- Modify: `internal/mcp/server.go`
- Modify: `internal/mcp/auth_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/mcp/auth_test.go`, add at the end:

```go
func TestWorkerScopesStoredInContext(t *testing.T) {
	scopes := []string{auth.ScopeReadWorkers, auth.ScopeReadTasks}
	tok, _ := auth.GenerateWorkerToken(testSecret, "worker-scoped", scopes, time.Hour)
	r := gin.New()
	r.Use(mcp.JWTAuthMiddleware(testSecret))
	r.GET("/test", func(c *gin.Context) {
		raw, _ := c.Get(mcp.CtxKeyScopesKey)
		got, _ := raw.([]string)
		if len(got) != 2 || got[0] != auth.ScopeReadWorkers || got[1] != auth.ScopeReadTasks {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "wrong scopes"})
			return
		}
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/mcp/... -run TestWorkerScopesStoredInContext -v
```

Expected: FAIL — compile error: `mcp.CtxKeyScopesKey` not defined

- [ ] **Step 3: Add CtxKeyScopesKey to auth.go and set scopes in middleware**

In `internal/mcp/auth.go`:

```go
const (
	CtxKeyTokenType = "mcp.token.type"
	CtxKeyWorkerID  = "mcp.token.worker_id"
	CtxKeyScopesKey = "mcp.token.scopes"
)

// JWTAuthMiddleware validates the MCP JWT and writes claims to gin.Context.
func JWTAuthMiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := c.GetHeader("X-API-Key")
		if tokenStr == "" {
			tokenStr = c.Query("api_key")
		}
		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		claims, err := auth.ValidateToken(tokenStr, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set(CtxKeyTokenType, claims.Type)
		c.Set(CtxKeyWorkerID, claims.WorkerID)
		c.Set(CtxKeyScopesKey, claims.Scopes)
		c.Next()
	}
}
```

- [ ] **Step 4: Propagate scopes into request context in server.go**

In `internal/mcp/server.go`, add a context key for scopes and update `workerIDContext`:

```go
// CtxScopesKey carries the caller's permission scopes through tool dispatch.
const CtxScopesKey ctxKey = CtxKeyScopesKey

func (s *MCPServer) workerIDContext(c *gin.Context) context.Context {
	ctx := context.WithValue(c.Request.Context(), CtxWorkerIDKey, c.GetString(CtxKeyWorkerID))
	scopes, _ := c.Get(CtxKeyScopesKey)
	return context.WithValue(ctx, CtxScopesKey, scopes)
}
```

- [ ] **Step 5: Run the test**

```bash
go test ./internal/mcp/... -run TestWorkerScopesStoredInContext -v
```

Expected: PASS

- [ ] **Step 6: Run all mcp tests**

```bash
go test ./internal/mcp/... -v
```

Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add internal/mcp/auth.go internal/mcp/server.go internal/mcp/auth_test.go
git commit -m "feat: propagate worker permission scopes through MCP auth middleware and request context"
```

---

## Task 6: MCP Tools — Scope Enforcement in beeCallTool

**Files:**
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/tools_test.go`

- [ ] **Step 1: Write failing tests**

In `internal/mcp/tools_test.go`, add a helper and tests at the end:

```go
func workerCtx(workerID string, scopes []string) context.Context {
	ctx := context.WithValue(context.Background(), mcp.CtxWorkerIDKey, workerID)
	return context.WithValue(ctx, mcp.CtxScopesKey, scopes)
}

func TestCheckWorkerScope_WorkerWithScope_CanCallScopedTool(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	ctx := workerCtx("wid-1", []string{"read:workers"})
	_, err := s.CallTool(ctx, utils.ListWorkers, mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestCheckWorkerScope_WorkerWithoutScope_CannotCallScopedTool(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	ctx := workerCtx("wid-1", nil) // no scopes
	_, err := s.CallTool(ctx, utils.ListWorkers, mustMarshal(t, map[string]any{}))
	if err == nil {
		t.Error("expected permission denied error, got nil")
	}
}

func TestCheckWorkerScope_WorkerWithWrongScope_CannotCallScopedTool(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	ctx := workerCtx("wid-1", []string{"read:tasks"}) // has tasks scope, not workers
	_, err := s.CallTool(ctx, utils.ListWorkers, mustMarshal(t, map[string]any{}))
	if err == nil {
		t.Error("expected permission denied error, got nil")
	}
}

func TestCheckWorkerScope_BeeToken_AlwaysAllowed(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	// Bee token: workerID is empty string in context
	ctx := context.Background() // no workerID key set = bee context
	_, err := s.CallTool(ctx, utils.ListWorkers, mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Errorf("bee token should always be allowed, got: %v", err)
	}
}

func TestCheckWorkerScope_WorkerToken_NonScopedTool_Unchanged(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	ctx := workerCtx("wid-1", nil) // no scopes
	// send_message is not in ToolScopeMap — existing behavior, worker can call it
	// (will fail for unrelated reasons — missing message — but NOT permission denied)
	_, err := s.CallTool(ctx, utils.SendMessage, mustMarshal(t, map[string]any{
		"message_id": "nonexistent",
		"content":    "test",
	}))
	// Should NOT be a permission denied error
	if err != nil && err.Error() == "permission denied: scope read:workers required" {
		t.Error("non-scoped tool should not return permission denied")
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/mcp/... -run "TestCheckWorkerScope" -v
```

Expected: FAIL — `mcp.CtxScopesKey` not exported yet from server.go (or tests reference it)

- [ ] **Step 3: Export CtxScopesKey from server.go**

The constant `CtxScopesKey` defined in Task 5 uses type `ctxKey` (unexported). The tests need to use it. Export `CtxScopesKey` as a typed constant accessible from `mcp_test` package by using the same string value approach as `CtxWorkerIDKey`.

In `internal/mcp/server.go`, confirm the exported constant is usable from `mcp_test`. Since `mcp_test` is in package `mcp_test` (external test package), it can access exported identifiers. Ensure `CtxScopesKey` is exported — the `const CtxScopesKey ctxKey = CtxKeyScopesKey` declaration is exported by name.

The test helper `workerCtx` uses `mcp.CtxScopesKey` with `context.WithValue`. Since the key type is `ctxKey` (unexported from mcp package), the test cannot construct this key directly. Instead, use the exported string constant `mcp.CtxKeyScopesKey` as the key in tests (matching what the middleware sets on gin context), and update `workerIDContext` to use the same string as the context key.

Update `server.go` to use `string` directly as the context key type for scopes (consistent with how workerID works):

```go
// Keep ctxKey type for CtxWorkerIDKey (existing pattern)
const CtxWorkerIDKey ctxKey = CtxKeyWorkerID

// Use the string constant directly for scopes so tests can construct contexts
// by referencing mcp.CtxKeyScopesKey as the key value.
// context.WithValue key type: use a dedicated exported type for test access.
type ctxScopesKey string
const CtxScopesKey ctxScopesKey = CtxKeyScopesKey

func (s *MCPServer) workerIDContext(c *gin.Context) context.Context {
	ctx := context.WithValue(c.Request.Context(), CtxWorkerIDKey, c.GetString(CtxKeyWorkerID))
	scopes, _ := c.Get(CtxKeyScopesKey)
	return context.WithValue(ctx, CtxScopesKey, scopes)
}
```

Update `workerCtx` in the test to use the exported key:

```go
func workerCtx(workerID string, scopes []string) context.Context {
	ctx := context.WithValue(context.Background(), mcp.CtxWorkerIDKey, workerID)
	return context.WithValue(ctx, mcp.CtxScopesKey, scopes)
}
```

- [ ] **Step 4: Add checkWorkerScope to tools.go**

At the top of `internal/mcp/tools.go`, add this helper function (before `beeCallTool`):

```go
// checkWorkerScope enforces scope-based access control for worker tokens.
// Bee tokens (empty workerID in context) are always allowed.
// Worker tokens must have the required scope for tools listed in auth.ToolScopeMap.
// Tools not in ToolScopeMap are unaffected (existing access rules apply).
func (s *MCPServer) checkWorkerScope(ctx context.Context, toolName string) error {
	workerID, _ := ctx.Value(CtxWorkerIDKey).(string)
	if workerID == "" {
		return nil // bee token: always allowed
	}
	requiredScope, ok := auth.ToolScopeMap[toolName]
	if !ok {
		return nil // tool not in scope map: follow existing rules
	}
	var scopes []string
	if v := ctx.Value(CtxScopesKey); v != nil {
		scopes, _ = v.([]string)
	}
	for _, sc := range scopes {
		if sc == requiredScope {
			return nil
		}
	}
	return fmt.Errorf("permission denied: scope %s required", requiredScope)
}
```

Add `"github.com/theopenbee/openbee/internal/infra/auth"` to the imports in `tools.go` if not already present.

- [ ] **Step 5: Call checkWorkerScope at the top of beeCallTool**

In `internal/mcp/tools.go`, update `beeCallTool`:

```go
func (s *MCPServer) beeCallTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	if err := s.checkWorkerScope(ctx, name); err != nil {
		return nil, err
	}
	switch name {
	// ... rest of switch unchanged ...
	}
}
```

- [ ] **Step 6: Run scope tests**

```bash
go test ./internal/mcp/... -run "TestCheckWorkerScope" -v
```

Expected: all PASS

- [ ] **Step 7: Run all mcp tests**

```bash
go test ./internal/mcp/... -v
```

Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_test.go internal/mcp/server.go
git commit -m "feat: enforce permission scopes in MCP tool dispatch for worker tokens"
```

---

## Task 7: CLI — Add --scopes Flag to ctl worker create/update

**Files:**
- Modify: `cmd/openbee/ctl_worker.go`

- [ ] **Step 1: Add scopes variables and flags**

In `cmd/openbee/ctl_worker.go`:

Add variables:

```go
var (
	workerCreateScopes string
	workerUpdateScopes string
)
```

Update `ctlWorkerCreateCmd.RunE` to pass `permission_scopes`:

```go
var ctlWorkerCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new worker",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"name": workerCreateName}
		if workerCreateDescription != "" {
			a["description"] = workerCreateDescription
		}
		if workerCreateMemory != "" {
			a["memory"] = workerCreateMemory
		}
		if workerCreateWorkDir != "" {
			a["work_dir"] = workerCreateWorkDir
		}
		if workerCreateDepartment != "" {
			a["department_ids"] = workerCreateDepartment
		}
		if workerCreateScopes != "" {
			a["permission_scopes"] = workerCreateScopes
		}
		return ctlRun(utils.CreateWorker, a)
	},
}
```

Update `ctlWorkerUpdateCmd.RunE` to pass `permission_scopes`:

```go
var ctlWorkerUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a worker (patch: omitted fields unchanged)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"worker_id": args[0]}
		if cmd.Flags().Changed("name") {
			a["name"] = workerUpdateName
		}
		if cmd.Flags().Changed("description") {
			a["description"] = workerUpdateDescription
		}
		if cmd.Flags().Changed("memory") {
			a["memory"] = workerUpdateMemory
		}
		if cmd.Flags().Changed("department") {
			a["department_ids"] = workerUpdateDepartment
		}
		if cmd.Flags().Changed("scopes") {
			a["permission_scopes"] = workerUpdateScopes
		}
		return ctlRun(utils.UpdateWorker, a)
	},
}
```

In the `init()` function, register the new flags:

```go
ctlWorkerCreateCmd.Flags().StringVar(&workerCreateScopes, "scopes", "", "Permission scopes (comma-separated, e.g. read:workers,read:tasks)")
ctlWorkerUpdateCmd.Flags().StringVar(&workerUpdateScopes, "scopes", "", "Permission scopes (comma-separated); replaces all scopes. Pass empty string to clear.")
```

- [ ] **Step 2: Build**

```bash
go build ./cmd/openbee/...
```

Expected: no output

- [ ] **Step 3: Update manager.CreateWorker to accept permissionScopes**

`toolCreateWorker` delegates to `manager.CreateWorker`. Update its signature in `internal/domain/worker/manager.go`:

```go
func (m *Manager) CreateWorker(
	name, description, memory string,
	workDir string,
	permissionScopes string,
) (model.Worker, error) {
	id := uuid.New().String()
	if workDir == "" {
		workDir = filepath.Join(m.workerBaseDir, id)
	}

	if err := os.MkdirAll(workDir, 0755); err != nil {
		return model.Worker{}, fmt.Errorf("create work dir: %w", err)
	}

	claudeMD := filepath.Join(workDir, "CLAUDE.md")
	if _, err := os.Stat(claudeMD); os.IsNotExist(err) {
		initialContent := claude.ImportLine + "\n"
		if err := os.WriteFile(claudeMD, []byte(initialContent), 0644); err != nil {
			return model.Worker{}, fmt.Errorf("create CLAUDE.md: %w", err)
		}
	}

	if err := claude.EnsureSystemRules(workDir, claude.RoleWorker, claude.WithName(name), claude.WithDescription(description), claude.WithMemory(memory)); err != nil {
		log.Error("ensure system rules", zap.String("op", "create"), zap.Error(err))
	}

	return m.workerStore.Create(model.Worker{
		ID:               id,
		Name:             name,
		Description:      description,
		Memory:           memory,
		WorkDir:          workDir,
		PermissionScopes: permissionScopes,
	})
}
```

- [ ] **Step 4: Update toolCreateWorker and toolUpdateWorker in tools.go**

In `internal/mcp/tools.go`, replace `toolCreateWorker` with:

```go
func (s *MCPServer) toolCreateWorker(args json.RawMessage) (any, error) {
	var params struct {
		Name             string `json:"name"`
		Description      string `json:"description"`
		Memory           string `json:"memory"`
		WorkDir          string `json:"work_dir"`
		DepartmentIDs    string `json:"department_ids"`
		PermissionScopes string `json:"permission_scopes"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	w, err := s.manager.CreateWorker(params.Name, params.Description, params.Memory, params.WorkDir, params.PermissionScopes)
	if err != nil {
		return nil, err
	}
	if params.DepartmentIDs != "" {
		if err := s.applyWorkerDepartments(w.ID, params.DepartmentIDs); err != nil {
			return nil, err
		}
	}
	return w, nil
}
```

Replace `toolUpdateWorker` with:

```go
func (s *MCPServer) toolUpdateWorker(args json.RawMessage) (any, error) {
	var params struct {
		WorkerID         string  `json:"worker_id"`
		Name             *string `json:"name"`
		Description      *string `json:"description"`
		Memory           *string `json:"memory"`
		DepartmentIDs    *string `json:"department_ids"`
		PermissionScopes *string `json:"permission_scopes"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}
	w, err := s.workerStore.GetByID(params.WorkerID)
	if err != nil {
		return nil, fmt.Errorf("worker not found: %w", err)
	}
	fieldsChanged := params.Name != nil || params.Description != nil || params.Memory != nil || params.PermissionScopes != nil
	if params.Name != nil {
		w.Name = *params.Name
	}
	if params.Description != nil {
		w.Description = *params.Description
	}
	if params.Memory != nil {
		w.Memory = *params.Memory
	}
	if params.PermissionScopes != nil {
		w.PermissionScopes = *params.PermissionScopes
	}
	if fieldsChanged {
		w, err = s.workerStore.Update(w)
		if err != nil {
			return nil, err
		}
	}
	if params.DepartmentIDs != nil {
		if err := s.applyWorkerDepartments(w.ID, *params.DepartmentIDs); err != nil {
			return nil, err
		}
	}
	return w, nil
}
```

- [ ] **Step 6: Build and run full test suite**

```bash
go build ./...
go test ./... -v 2>&1 | tail -30
```

Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add cmd/openbee/ctl_worker.go internal/mcp/tools.go internal/domain/worker/manager.go
git commit -m "feat: add --scopes flag to ctl worker create/update and handle permission_scopes in tool handlers"
```

---

## Task 8: Update openbee-worker SKILL.md

**Files:**
- Modify: `internal/infra/skillinstall/skills/openbee-worker/SKILL.md`

- [ ] **Step 1: Append read-only commands section**

At the end of `internal/infra/skillinstall/skills/openbee-worker/SKILL.md`, after the `### message subcommand` section, add:

````markdown
### Read-Only Query Commands (Requires Permission Scope)

The following commands are available only if the administrator has granted the corresponding
permission scope to this worker. The worker token in `OPENBEE_API_KEY` is used automatically —
no additional configuration is needed.

If a command returns a "permission denied" error, the worker has not been granted the required scope.
Ask the administrator to run `openbee ctl worker update <id> --scopes <scope>`.

**Requires `read:workers` scope:**

```bash
openbee ctl worker list                        # List all workers
openbee ctl worker list --department <id>      # Filter by department ID or name
openbee ctl worker get <id>                    # Get worker details by ID
openbee ctl worker status <id>                 # Get worker current status (idle/working/error)
```

**Requires `read:departments` scope:**

```bash
openbee ctl department list                    # List all departments (tree structure)
openbee ctl department get <id|name>           # Get department details by ID or name
```

**Requires `read:tasks` scope:**

```bash
openbee ctl task list --worker-id <id>         # List tasks assigned to a worker
openbee ctl task list --status pending         # Filter tasks by status
openbee ctl task list --session-key <key>      # Filter tasks by session key
```
````

- [ ] **Step 2: Commit**

```bash
git add internal/infra/skillinstall/skills/openbee-worker/SKILL.md
git commit -m "docs: add read-only ctl commands section to openbee-worker skill"
```

---

## Task 9: Final Verification

- [ ] **Step 1: Run the full test suite**

```bash
go test ./... -count=1
```

Expected: all PASS, no failures

- [ ] **Step 2: Build the binary**

```bash
go build ./cmd/openbee/
```

Expected: no output

- [ ] **Step 3: Verify ctl help shows --scopes flag**

```bash
./openbee ctl worker create --help
./openbee ctl worker update --help
```

Expected: both commands show `--scopes` flag in their help output

- [ ] **Step 4: Final commit if any loose changes**

```bash
git status
# If clean: done. If not, stage and commit remaining changes.
```
