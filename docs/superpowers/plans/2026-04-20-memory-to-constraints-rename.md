# memory → constraints Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the `memory` field in `bee_workers` to `constraints` across all layers — database, Go backend, REST API, MCP tools, frontend, and i18n — to eliminate confusion with AI agent memory systems.

**Architecture:** A single migration renames the DB column; Go identifiers, SQL strings, and JSON tags are updated in place; frontend state variables and API payload keys follow. All changes are in a single commit per task. No backward-compatibility shims.

**Tech Stack:** Go 1.23+, SQLite (modernc.org/sqlite v1.46.1), Gin, React + TypeScript, i18next

---

## File Map

| File | Change |
|------|--------|
| `internal/infra/store/db.go` | Add migration 39 (RENAME COLUMN) |
| `internal/infra/model/worker.go` | `Memory` → `Constraints`, update json/db tags |
| `internal/infra/store/worker_store.go` | SQL strings + `scanWorker` field |
| `internal/domain/worker/manager.go` | `CreateWorkerParams.Memory` → `.Constraints` |
| `internal/domain/task/dispatcher.go` | `worker.Memory` → `worker.Constraints` |
| `internal/ai/rules.go` | param + section header |
| `internal/ai/rules_test.go` | assertion string |
| `internal/api/worker_handler.go` | request structs + JSON tags |
| `internal/mcp/tools.go` | tool params + field references |
| `internal/mcp/tools_test.go` | test field names + assertions |
| `web/src/pages/worker-detail.tsx` | state vars + API payload key |
| `web/src/components/create-worker-sheet.tsx` | form field + API payload key |
| `web/src/locales/en.json` | 5 copy strings |
| `web/src/locales/zh.json` | 5 copy strings |

---

### Task 1: Database Migration

**Files:**
- Modify: `internal/infra/store/db.go`

- [ ] **Step 1: Add migration 39**

In `internal/infra/store/db.go`, append to the `migrations` slice after the existing version 38 entry:

```go
{
    version: 39,
    name:    "rename_bee_workers_memory_to_constraints",
    sql:     `ALTER TABLE bee_workers RENAME COLUMN memory TO constraints`,
},
```

- [ ] **Step 2: Verify the migration compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go build ./internal/infra/store/...
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/infra/store/db.go
git commit -m "feat: add migration 39 to rename bee_workers.memory to constraints"
```

---

### Task 2: Go Model

**Files:**
- Modify: `internal/infra/model/worker.go`

- [ ] **Step 1: Update the Worker struct field**

In `internal/infra/model/worker.go`, change line 15:

```go
// Before
Memory string `json:"memory" db:"memory"`

// After
Constraints string `json:"constraints" db:"constraints"`
```

- [ ] **Step 2: Verify it compiles (will fail on dependents — expected)**

```bash
go build ./internal/infra/model/...
```

Expected: success (model package itself compiles; downstream errors appear later).

- [ ] **Step 3: Commit**

```bash
git add internal/infra/model/worker.go
git commit -m "feat: rename Worker.Memory to Worker.Constraints with updated json/db tags"
```

---

### Task 3: Store Layer

**Files:**
- Modify: `internal/infra/store/worker_store.go`

- [ ] **Step 1: Update SQL constants and INSERT**

Replace both SQL constants (lines 42–43) and the INSERT statement (lines 29–33):

```go
const (
    workerColumns        = `id, name, description, constraints, work_dir, engine, status, permission_scopes, created_at, updated_at`
    workerColumnsAliased = `w.id, w.name, w.description, w.constraints, w.work_dir, w.engine, w.status, w.permission_scopes, w.created_at, w.updated_at`
)
```

In the `Create` method, update the INSERT (lines 29–33):

```go
_, err := s.db.Exec(
    `INSERT INTO bee_workers (id, name, description, constraints, work_dir, engine, status, permission_scopes, created_at, updated_at)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    w.ID, w.Name, w.Description, w.Constraints, w.WorkDir, w.Engine,
    w.Status, w.PermissionScopes, w.CreatedAt, w.UpdatedAt,
)
```

- [ ] **Step 2: Update scanWorker**

In `scanWorker` (lines 46–56), change `&w.Memory` → `&w.Constraints`:

```go
func scanWorker(scanner interface{ Scan(...any) error }) (model.Worker, error) {
    var w model.Worker
    err := scanner.Scan(
        &w.ID, &w.Name, &w.Description, &w.Constraints,
        &w.WorkDir, &w.Engine, &w.Status, &w.PermissionScopes, &w.CreatedAt, &w.UpdatedAt,
    )
    if err != nil {
        return model.Worker{}, err
    }
    return w, nil
}
```

- [ ] **Step 3: Update UPDATE statement**

In the `Update` method (lines 191–203), change the SET clause:

```go
_, err := s.db.Exec(
    `UPDATE bee_workers SET name=?, description=?, constraints=?, work_dir=?, engine=?, status=?, permission_scopes=?, updated_at=?
     WHERE id=?`,
    w.Name, w.Description, w.Constraints, w.WorkDir, w.Engine,
    w.Status, w.PermissionScopes, w.UpdatedAt, w.ID,
)
```

- [ ] **Step 4: Build store package**

```bash
go build ./internal/infra/store/...
```

Expected: success.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/worker_store.go
git commit -m "feat: update worker store SQL to use constraints column"
```

---

### Task 4: AI Rules — Function Signature and Prompt Header

**Files:**
- Modify: `internal/ai/rules.go`
- Modify: `internal/ai/rules_test.go`

- [ ] **Step 1: Update existing test assertion to target new header**

In `internal/ai/rules_test.go`, `TestWorkerPersona_Full` (lines 31–51), change the header assertion (line 42):

```go
// Before
if !strings.Contains(got, "## Memory Constraints") {
    t.Errorf("missing memory header, got: %q", got)
}

// After
if !strings.Contains(got, "## Work Constraints") {
    t.Errorf("missing constraints header, got: %q", got)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/ai/... -run TestWorkerPersona_Full -v
```

Expected: FAIL — `missing constraints header`.

- [ ] **Step 3: Update rules.go**

In `internal/ai/rules.go`, update `WorkerPersona` (lines 6–18):

```go
func WorkerPersona(name, description, constraints string) string {
    s := "You are a Worker in an AI team.\n"
    if name != "" {
        s += fmt.Sprintf("Name: %s\n", name)
    }
    if description != "" {
        s += fmt.Sprintf("Description: %s\n", description)
    }
    if constraints != "" {
        s += fmt.Sprintf("\n## Work Constraints\n%s\n", constraints)
    }
    return s
}
```

- [ ] **Step 4: Run all AI tests**

```bash
go test ./internal/ai/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/rules.go internal/ai/rules_test.go
git commit -m "feat: rename WorkerPersona memory param to constraints, update prompt header"
```

---

### Task 5: Domain Layer — manager.go and dispatcher.go

**Files:**
- Modify: `internal/domain/worker/manager.go`
- Modify: `internal/domain/task/dispatcher.go`

- [ ] **Step 1: Update CreateWorkerParams and its usage**

In `internal/domain/worker/manager.go`, rename the field in `CreateWorkerParams` (lines 102–109):

```go
type CreateWorkerParams struct {
    Name             string
    Description      string
    Constraints      string
    WorkDir          string
    PermissionScopes string
    Engine           string
}
```

Update the assignment in `CreateWorker` (line 125):

```go
workerModel := model.Worker{
    ID:               id,
    Name:             p.Name,
    Description:      p.Description,
    Constraints:      p.Constraints,
    WorkDir:          p.WorkDir,
    Engine:           p.Engine,
    PermissionScopes: p.PermissionScopes,
}
```

- [ ] **Step 2: Update dispatcher.go**

In `internal/domain/task/dispatcher.go` line 333, update the `WorkerPersona` call:

```go
persona := ai.WorkerPersona(worker.Name, worker.Description, worker.Constraints)
```

- [ ] **Step 3: Build domain packages**

```bash
go build ./internal/domain/...
```

Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/worker/manager.go internal/domain/task/dispatcher.go
git commit -m "feat: rename CreateWorkerParams.Memory to Constraints, update dispatcher"
```

---

### Task 6: REST API Handler

**Files:**
- Modify: `internal/api/worker_handler.go`

- [ ] **Step 1: Update createWorkerRequest**

In `internal/api/worker_handler.go`, change the `createWorkerRequest` struct (lines 13–20) and the PATCH request struct (lines 118–124):

```go
type createWorkerRequest struct {
    Name             string `json:"name" binding:"required"`
    Engine           string `json:"engine"`
    Description      string `json:"description"`
    Constraints      string `json:"constraints"`
    WorkDir          string `json:"work_dir"`
    PermissionScopes string `json:"permission_scopes"`
}
```

Update the `CreateWorker` call in `Create` (lines 49–56):

```go
w, err := h.manager.CreateWorker(worker.CreateWorkerParams{
    Name:             req.Name,
    Engine:           req.Engine,
    Description:      req.Description,
    Constraints:      req.Constraints,
    WorkDir:          req.WorkDir,
    PermissionScopes: req.PermissionScopes,
})
```

- [ ] **Step 2: Update PATCH request struct and handler**

In `Update` (lines 118–137), change the inline request struct and its usage:

```go
var req struct {
    Name             *string `json:"name"`
    Description      *string `json:"description"`
    Constraints      *string `json:"constraints"`
    PermissionScopes *string `json:"permission_scopes"`
    Engine           *string `json:"engine"`
}
// ...
if req.Constraints != nil {
    w.Constraints = *req.Constraints
}
```

- [ ] **Step 3: Build api package**

```bash
go build ./internal/api/...
```

Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/api/worker_handler.go
git commit -m "feat: update REST API worker handler to use constraints field"
```

---

### Task 7: MCP Tools

**Files:**
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/tools_test.go`

- [ ] **Step 1: Update test first (TDD)**

In `internal/mcp/tools_test.go` `TestCallTool_UpdateWorker` (lines 158–181), change the tool call and assertions:

```go
result, err := s.CallTool(context.Background(), "update_worker", mustMarshal(t, map[string]any{
    "worker_id":   w.ID,
    "name":        "NewName",
    "constraints": "New constraints",
}))
// ...
if updated.Constraints != "New constraints" {
    t.Errorf("expected new constraints, got %s", updated.Constraints)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/mcp/... -run TestCallTool_UpdateWorker -v
```

Expected: FAIL (field mismatch or compile error).

- [ ] **Step 3: Update toolCreateWorker params struct**

In `internal/mcp/tools.go` `toolCreateWorker` (lines 237–276), rename the JSON tag and field:

```go
var params struct {
    Name             string `json:"name"`
    Description      string `json:"description"`
    Constraints      string `json:"constraints"`
    WorkDir          string `json:"work_dir"`
    Engine           string `json:"engine"`
    DepartmentIDs    string `json:"department_ids"`
    PermissionScopes string `json:"permission_scopes"`
}
// ...
w, err := s.manager.CreateWorker(worker.CreateWorkerParams{
    Name:             params.Name,
    Description:      params.Description,
    Constraints:      params.Constraints,
    WorkDir:          params.WorkDir,
    Engine:           params.Engine,
    PermissionScopes: params.PermissionScopes,
})
```

- [ ] **Step 4: Update toolUpdateWorker params struct**

In `internal/mcp/tools.go` `toolUpdateWorker` (lines 278–330), rename the field and its usages:

```go
var params struct {
    WorkerID         string  `json:"worker_id"`
    Name             *string `json:"name"`
    Description      *string `json:"description"`
    Constraints      *string `json:"constraints"`
    Engine           *string `json:"engine"`
    DepartmentIDs    *string `json:"department_ids"`
    PermissionScopes *string `json:"permission_scopes"`
}
// ...
fieldsChanged := params.Name != nil || params.Description != nil || params.Constraints != nil || params.Engine != nil || params.PermissionScopes != nil
// ...
if params.Constraints != nil {
    w.Constraints = *params.Constraints
}
```

- [ ] **Step 5: Run all MCP tests**

```bash
go test ./internal/mcp/... -v
```

Expected: all PASS.

- [ ] **Step 6: Build entire backend to confirm no remaining compile errors**

```bash
go build ./...
```

Expected: success.

- [ ] **Step 7: Run all Go tests**

```bash
go test ./...
```

Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_test.go
git commit -m "feat: update MCP tools to use constraints parameter"
```

---

### Task 8: Frontend — i18n Copy

**Files:**
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`

- [ ] **Step 1: Update en.json**

In `web/src/locales/en.json`, update both `createWorker` section (lines 98–100) and `workerDetail` section (lines 157–160):

```json
"createWorker": {
  ...
  "memory": "Work Constraints",
  "memoryPlaceholder": "The work constraints for this worker...",
  "memoryHelper": "Work constraints injected into the worker at the start of every session.",
  ...
}
```

```json
"workerDetail": {
  ...
  "memory": "Work Constraints",
  "noMemory": "No work constraints configured",
  "editMemory": "Edit work constraints",
  ...
}
```

- [ ] **Step 2: Update zh.json**

In `web/src/locales/zh.json`, update both sections (lines 98–100 and 157–160):

```json
"createWorker": {
  ...
  "memory": "工作约束",
  "memoryPlaceholder": "这个员工的工作约束...",
  "memoryHelper": "每次会话开始时注入到员工中的工作约束。",
  ...
}
```

```json
"workerDetail": {
  ...
  "memory": "工作约束",
  "noMemory": "未配置工作约束",
  "editMemory": "编辑工作约束",
  ...
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/locales/en.json web/src/locales/zh.json
git commit -m "feat: update i18n copy from static memory to work constraints"
```

---

### Task 9: Frontend — worker-detail.tsx and create-worker-sheet.tsx

**Files:**
- Modify: `web/src/pages/worker-detail.tsx`
- Modify: `web/src/components/create-worker-sheet.tsx`

- [ ] **Step 1: Update worker-detail.tsx state variables and API payload**

Search for all occurrences of `Memory`, `memory`, `isEditingMemory`, `editMemory` in `web/src/pages/worker-detail.tsx` and rename:

- `isEditingMemory` → `isEditingConstraints`
- `editMemory` → `editConstraints`
- `setIsEditingMemory` → `setIsEditingConstraints`
- `setEditMemory` → `setEditConstraints`
- API payload key `memory:` → `constraints:`
- Any reference to `worker.memory` (from API response) stays `worker.constraints` — the model field name in JSON response has changed

Run a targeted replacement:

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web/src/pages
sed -i '' \
  -e 's/isEditingMemory/isEditingConstraints/g' \
  -e 's/setIsEditingMemory/setIsEditingConstraints/g' \
  -e 's/editMemory/editConstraints/g' \
  -e 's/setEditMemory/setEditConstraints/g' \
  -e 's/"memory":/\"constraints\":/g' \
  -e 's/worker\.memory/worker.constraints/g' \
  worker-detail.tsx
```

- [ ] **Step 2: Update create-worker-sheet.tsx**

Search for `memory` form field references and API payload in `web/src/components/create-worker-sheet.tsx`:

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web/src/components
sed -i '' \
  -e 's/\.memory\b/.constraints/g' \
  -e 's/"memory":/\"constraints\":/g' \
  -e 's/memory:/constraints:/g' \
  create-worker-sheet.tsx
```

- [ ] **Step 3: TypeScript build check**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
npm run build 2>&1 | tail -20
```

Expected: build succeeds with no TypeScript errors. If there are errors, fix them before committing (the sed replacements may have missed a case — inspect the diff with `git diff web/src/`).

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/worker-detail.tsx web/src/components/create-worker-sheet.tsx
git commit -m "feat: update frontend components to use constraints field"
```

---

### Task 10: Final Verification

- [ ] **Step 1: Run all Go tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./...
```

Expected: all PASS.

- [ ] **Step 2: TypeScript build**

```bash
cd web && npm run build
```

Expected: success, no errors.

- [ ] **Step 3: Smoke test the migration on a copy of the DB**

```bash
sqlite3 /tmp/smoke.db << 'SQL'
CREATE TABLE bee_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')));
CREATE TABLE bee_workers (id TEXT PRIMARY KEY, name TEXT NOT NULL, work_dir TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'idle', description TEXT NOT NULL DEFAULT '', memory TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);
INSERT INTO bee_workers VALUES ('1','test','/tmp','idle','','my memory',1,1);
ALTER TABLE bee_workers RENAME COLUMN memory TO constraints;
SELECT constraints FROM bee_workers WHERE id='1';
SQL
```

Expected output: `my memory` (data preserved after rename).

- [ ] **Step 4: Review git log**

```bash
git log --oneline -10
```

Confirm all task commits are present and correctly described.
