# Department Management CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `openbee ctl department` CRUD subcommands and extend `openbee ctl worker` with department-related flags (`--department`, `--no-recursive`), backed by new MCP tools.

**Architecture:** New department MCP tools are added to the existing `internal/ai/mcp/tools.go` file following the current pattern. A `departmentStore` field is added to `MCPServer` and injected via `NewBeeServer`. Existing worker tools (`list_workers`, `create_worker`, `update_worker`) are extended with optional department parameters. CLI commands in `cmd/openbee/` call these tools via `ctlRun()` with no new network patterns.

**Tech Stack:** Go, Cobra (CLI), SQLite via `database/sql`, existing `store.DepartmentStore`

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/infra/utils/toolnames.go` | Modify | Add 5 department tool name constants |
| `internal/ai/mcp/server.go` | Modify | Add `departmentStore` field; update `NewBeeServer` signature |
| `internal/ai/mcp/tools.go` | Modify | Add department tool schemas, handlers, helpers; extend 3 worker tools |
| `internal/app/app.go` | Modify | Pass `departmentStore` to `NewBeeServer` |
| `internal/ai/mcp/tools_test.go` | Modify | Update `setupMCPServer*` helpers; add department tool tests |
| `cmd/openbee/ctl_department.go` | Create | `department list/get/create/update/delete` CLI commands |
| `cmd/openbee/ctl_worker.go` | Modify | Add `--department`/`--no-recursive` to `list`, `create`, `update` |

---

## Task 1: Add department tool name constants

**Files:**
- Modify: `internal/infra/utils/toolnames.go`

- [ ] **Step 1: Add the 5 constants**

Open `internal/infra/utils/toolnames.go` and append after the last existing constant:

```go
const (
	ListWorkers         = "list_workers"
	GetWorker           = "get_worker"
	CreateWorker        = "create_worker"
	UpdateWorker        = "update_worker"
	DeleteWorker        = "delete_worker"
	CreateTask          = "create_task"
	ListTasks           = "list_tasks"
	CancelTask          = "cancel_task"
	SendMessage         = "send_message"
	ClearSession        = "clear_session"
	GetWorkerStatus     = "get_worker_status"
	GetSystemOverview   = "get_system_overview"
	ListBeeExecutions   = "list_bee_executions"
	SaveMemory          = "save_memory"
	GetMemory           = "get_memory"
	DeleteMemory        = "delete_memory"
	ListSessionContexts = "list_session_contexts"
	ClearWorkerSession  = "clear_worker_session"
	ListDepartments     = "list_departments"
	GetDepartment       = "get_department"
	CreateDepartment    = "create_department"
	UpdateDepartment    = "update_department"
	DeleteDepartment    = "delete_department"
)
```

- [ ] **Step 2: Build to confirm no errors**

```bash
go build ./internal/infra/utils/...
```
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add internal/infra/utils/toolnames.go
git commit -m "feat: add department MCP tool name constants"
```

---

## Task 2: Add `departmentStore` to `MCPServer` and update constructor

**Files:**
- Modify: `internal/ai/mcp/server.go`
- Modify: `internal/app/app.go`
- Modify: `internal/ai/mcp/tools_test.go`

- [ ] **Step 1: Add `departmentStore` field to `MCPServer` struct**

In `internal/ai/mcp/server.go`, the struct currently ends with:
```go
	memoryStore    *store.MemoryStore
	sessionStore   *store.SessionStore

	mu              sync.Mutex
	sessions        map[string]chan rpcResponse
	workerNameCache sync.Map
}
```

Change it to:
```go
	memoryStore     *store.MemoryStore
	sessionStore    *store.SessionStore
	departmentStore *store.DepartmentStore

	mu              sync.Mutex
	sessions        map[string]chan rpcResponse
	workerNameCache sync.Map
}
```

- [ ] **Step 2: Update `NewBeeServer` signature and body**

The current signature ends with `sessionStore *store.SessionStore`. Add `ds *store.DepartmentStore` as the last parameter, and set the field:

```go
func NewBeeServer(
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
	ds *store.DepartmentStore,
) *MCPServer {
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
	return s
}
```

- [ ] **Step 3: Update the call in `internal/app/app.go`**

Find line 115:
```go
beeMCPSrv := mcp.NewBeeServer(s.workerStore, mgr, s.taskStore, s.msgStore, sendersByPlatform, mgr, disp, s.execStore, s.memoryStore, s.sessionStore)
```

Change to:
```go
beeMCPSrv := mcp.NewBeeServer(s.workerStore, mgr, s.taskStore, s.msgStore, sendersByPlatform, mgr, disp, s.execStore, s.memoryStore, s.sessionStore, s.departmentStore)
```

- [ ] **Step 4: Update test helpers in `internal/ai/mcp/tools_test.go`**

There are 3 calls to `mcp.NewBeeServer` in tests. Update each by appending `store.NewDepartmentStore(db)`:

```go
// In setupMCPServerWithMessaging (line ~39):
return mcp.NewBeeServer(ws, mgr, ts, ms, senders, nil, nil, es, store.NewMemoryStore(db), store.NewSessionStore(db), store.NewDepartmentStore(db))

// In setupMCPServerWithSender (line ~218):
return mcp.NewBeeServer(ws, mgr, ts, ms, senders, nil, nil, es, store.NewMemoryStore(db), store.NewSessionStore(db), store.NewDepartmentStore(db)), db

// In setupMCPServerWithClear (line ~489):
return mcp.NewBeeServer(ws, mgr, ts, ms, senders, stopper, clearer, es, store.NewMemoryStore(db), store.NewSessionStore(db), store.NewDepartmentStore(db)), db, stopper, clearer
```

- [ ] **Step 5: Build to confirm no compile errors**

```bash
go build ./...
```
Expected: no output (success)

- [ ] **Step 6: Run existing tests to confirm nothing is broken**

```bash
go test ./internal/ai/mcp/... -v -count=1 2>&1 | tail -20
```
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add internal/ai/mcp/server.go internal/app/app.go internal/ai/mcp/tools_test.go
git commit -m "feat: inject departmentStore into MCPServer"
```

---

## Task 3: Add helper functions to `tools.go`

**Files:**
- Modify: `internal/ai/mcp/tools.go`

These helpers are used by all department tools and the extended worker tools.

- [ ] **Step 1: Add `splitAndTrim`, `ancestorPath`, `resolveDepartmentID`, `collectDescendantIDs` to `tools.go`**

Add the following block at the end of `internal/ai/mcp/tools.go` (before the final closing, or at the bottom of the file):

```go
// splitAndTrim splits a comma-separated string and trims whitespace from each part,
// returning only non-empty parts.
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// resolveDepartmentID resolves an ID-or-name string to a department ID.
// It tries by ID first; if not found, falls back to name match.
// Returns an error if no match or if the name is ambiguous.
func (s *MCPServer) resolveDepartmentID(idOrName string) (string, error) {
	if _, err := s.departmentStore.GetByID(idOrName); err == nil {
		return idOrName, nil
	}
	all, err := s.departmentStore.ListAll()
	if err != nil {
		return "", fmt.Errorf("list departments: %w", err)
	}
	var matches []model.Department
	for _, d := range all {
		if d.Name == idOrName {
			matches = append(matches, d)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("department %q not found", idOrName)
	case 1:
		return matches[0].ID, nil
	default:
		deptMap := make(map[string]model.Department, len(all))
		for _, d := range all {
			deptMap[d.ID] = d
		}
		paths := make([]string, len(matches))
		for i, m := range matches {
			paths[i] = departmentAncestorPath(deptMap, m)
		}
		return "", fmt.Errorf("department name %q is ambiguous, matches: %s; use an ID instead",
			idOrName, strings.Join(paths, ", "))
	}
}

// resolveDepartmentIDs resolves a slice of ID-or-name strings to department IDs.
func (s *MCPServer) resolveDepartmentIDs(idOrNames []string) ([]string, error) {
	ids := make([]string, 0, len(idOrNames))
	for _, v := range idOrNames {
		id, err := s.resolveDepartmentID(v)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// departmentAncestorPath builds "grandparent > parent > dept" for display.
func departmentAncestorPath(deptMap map[string]model.Department, d model.Department) string {
	var parts []string
	cur := d
	for {
		parts = append([]string{cur.Name}, parts...)
		if cur.ParentID == nil {
			break
		}
		parent, ok := deptMap[*cur.ParentID]
		if !ok {
			break
		}
		cur = parent
	}
	return strings.Join(parts, " > ")
}

// collectDescendantIDs returns rootID plus all descendant department IDs via DFS.
func collectDescendantIDs(all []model.Department, rootID string) []string {
	childrenMap := make(map[string][]string)
	for _, d := range all {
		if d.ParentID != nil {
			childrenMap[*d.ParentID] = append(childrenMap[*d.ParentID], d.ID)
		}
	}
	var ids []string
	var dfs func(id string)
	dfs = func(id string) {
		ids = append(ids, id)
		for _, childID := range childrenMap[id] {
			dfs(childID)
		}
	}
	dfs(rootID)
	return ids
}

// listWorkersRecursive returns workers in deptID and all its descendant departments.
func (s *MCPServer) listWorkersRecursive(deptID string) ([]model.Worker, error) {
	all, err := s.departmentStore.ListAll()
	if err != nil {
		return nil, fmt.Errorf("list departments: %w", err)
	}
	deptIDs := collectDescendantIDs(all, deptID)

	workerIDSet := make(map[string]struct{})
	for _, id := range deptIDs {
		wids, err := s.departmentStore.GetDepartmentWorkerIDs(id)
		if err != nil {
			return nil, fmt.Errorf("get department workers: %w", err)
		}
		for _, wid := range wids {
			workerIDSet[wid] = struct{}{}
		}
	}

	workerIDs := make([]string, 0, len(workerIDSet))
	for wid := range workerIDSet {
		workerIDs = append(workerIDs, wid)
	}
	return s.workerStore.GetByIDs(workerIDs)
}
```

Also add `"strings"` to the import block in `tools.go` if not already present.

- [ ] **Step 2: Build to confirm no compile errors**

```bash
go build ./internal/ai/mcp/...
```
Expected: no output

- [ ] **Step 3: Write a failing test for `resolveDepartmentID` in `tools_test.go`**

Add at the end of `internal/ai/mcp/tools_test.go`:

```go
func TestResolveDepartmentID_ByID(t *testing.T) {
	s, db := setupMCPServerWithSender(t)
	ds := store.NewDepartmentStore(db)

	dept, err := ds.Create(model.Department{Name: "Engineering"})
	if err != nil {
		t.Fatalf("create dept: %v", err)
	}

	result, err := s.CallTool(context.Background(), "get_department",
		mustMarshal(t, map[string]any{"id": dept.ID}))
	if err != nil {
		t.Fatalf("get_department by ID: %v", err)
	}
	got, ok := result.(model.Department)
	if !ok {
		t.Fatalf("expected model.Department, got %T", result)
	}
	if got.ID != dept.ID {
		t.Errorf("expected ID %s, got %s", dept.ID, got.ID)
	}
}

func TestResolveDepartmentID_ByName(t *testing.T) {
	s, db := setupMCPServerWithSender(t)
	ds := store.NewDepartmentStore(db)

	_, err := ds.Create(model.Department{Name: "Marketing"})
	if err != nil {
		t.Fatalf("create dept: %v", err)
	}

	result, err := s.CallTool(context.Background(), "get_department",
		mustMarshal(t, map[string]any{"id": "Marketing"}))
	if err != nil {
		t.Fatalf("get_department by name: %v", err)
	}
	got, ok := result.(model.Department)
	if !ok {
		t.Fatalf("expected model.Department, got %T", result)
	}
	if got.Name != "Marketing" {
		t.Errorf("expected name Marketing, got %s", got.Name)
	}
}

func TestResolveDepartmentID_NotFound(t *testing.T) {
	s, _ := setupMCPServerWithSender(t)
	_, err := s.CallTool(context.Background(), "get_department",
		mustMarshal(t, map[string]any{"id": "nonexistent"}))
	if err == nil {
		t.Fatal("expected error for nonexistent department, got nil")
	}
}
```

- [ ] **Step 4: Run the tests to confirm they fail (get_department not yet implemented)**

```bash
go test ./internal/ai/mcp/... -run TestResolveDepartment -v 2>&1
```
Expected: FAIL with `unknown tool: get_department`

- [ ] **Step 5: Commit the helpers (tests will be green after Task 4)**

```bash
git add internal/ai/mcp/tools.go internal/ai/mcp/tools_test.go
git commit -m "feat: add department resolution helpers and recursive worker query"
```

---

## Task 4: Add `list_departments` and `get_department` MCP tools

**Files:**
- Modify: `internal/ai/mcp/tools.go`

- [ ] **Step 1: Add schemas to `beeToolSchemas()`**

At the end of the `beeToolSchemas()` return slice (before the closing `}`), add:

```go
		{
			Name:        utils.ListDepartments,
			Description: "List all departments as a tree structure",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        utils.GetDepartment,
			Description: "Get a department by ID or name",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": map[string]string{"type": "string", "description": "Department ID or name"},
				},
			},
		},
```

- [ ] **Step 2: Add handlers and cases to `beeCallTool`**

In the `switch` statement in `beeCallTool`, add before the `default` case:

```go
	case utils.ListDepartments:
		return s.toolListDepartments(args)
	case utils.GetDepartment:
		return s.toolGetDepartment(args)
```

- [ ] **Step 3: Add handler implementations**

Add at the end of `tools.go` (before the helper functions added in Task 3):

```go
func (s *MCPServer) toolListDepartments(_ json.RawMessage) (any, error) {
	all, err := s.departmentStore.ListAll()
	if err != nil {
		return nil, fmt.Errorf("list departments: %w", err)
	}
	tree := s.departmentStore.BuildTree(all)
	if tree == nil {
		tree = []model.DepartmentTree{}
	}
	return tree, nil
}

func (s *MCPServer) toolGetDepartment(args json.RawMessage) (any, error) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	deptID, err := s.resolveDepartmentID(params.ID)
	if err != nil {
		return nil, err
	}
	return s.departmentStore.GetByID(deptID)
}
```

- [ ] **Step 4: Run the resolver tests to confirm they now pass**

```bash
go test ./internal/ai/mcp/... -run TestResolveDepartment -v 2>&1
```
Expected: all PASS

- [ ] **Step 5: Write a failing test for `list_departments`**

Add to `tools_test.go`:

```go
func TestCallTool_ListDepartments_Empty(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	result, err := s.CallTool(context.Background(), "list_departments", mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Fatalf("list_departments: %v", err)
	}
	tree, ok := result.([]model.DepartmentTree)
	if !ok {
		t.Fatalf("expected []model.DepartmentTree, got %T", result)
	}
	if len(tree) != 0 {
		t.Errorf("expected empty tree, got %d roots", len(tree))
	}
}

func TestCallTool_ListDepartments_Tree(t *testing.T) {
	s, db := setupMCPServerWithSender(t)
	ds := store.NewDepartmentStore(db)

	parent, _ := ds.Create(model.Department{Name: "R&D"})
	_, _ = ds.Create(model.Department{Name: "Frontend", ParentID: &parent.ID})
	_, _ = ds.Create(model.Department{Name: "Backend", ParentID: &parent.ID})

	result, err := s.CallTool(context.Background(), "list_departments", mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Fatalf("list_departments: %v", err)
	}
	tree, ok := result.([]model.DepartmentTree)
	if !ok {
		t.Fatalf("expected []model.DepartmentTree, got %T", result)
	}
	if len(tree) != 1 {
		t.Fatalf("expected 1 root, got %d", len(tree))
	}
	if tree[0].Name != "R&D" {
		t.Errorf("expected root name R&D, got %s", tree[0].Name)
	}
	if len(tree[0].Children) != 2 {
		t.Errorf("expected 2 children, got %d", len(tree[0].Children))
	}
}
```

- [ ] **Step 6: Run the new tests**

```bash
go test ./internal/ai/mcp/... -run "TestCallTool_ListDepartments|TestResolveDepartment" -v 2>&1
```
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add internal/ai/mcp/tools.go internal/ai/mcp/tools_test.go
git commit -m "feat: add list_departments and get_department MCP tools"
```

---

## Task 5: Add `create_department`, `update_department`, `delete_department` MCP tools

**Files:**
- Modify: `internal/ai/mcp/tools.go`
- Modify: `internal/ai/mcp/tools_test.go`

- [ ] **Step 1: Add schemas to `beeToolSchemas()`**

Append after the `get_department` schema added in Task 4:

```go
		{
			Name:        utils.CreateDepartment,
			Description: "Create a new department",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]any{
					"name":       map[string]string{"type": "string", "description": "Department name"},
					"parent_id":  map[string]string{"type": "string", "description": "Parent department ID or name"},
					"sort_order": map[string]string{"type": "integer", "description": "Display sort order"},
				},
			},
		},
		{
			Name:        utils.UpdateDepartment,
			Description: "Update a department (patch semantics: omitted fields unchanged). Setting parent_id moves the department; cannot move to root level.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id":         map[string]string{"type": "string", "description": "Department ID or name"},
					"name":       map[string]string{"type": "string", "description": "New name"},
					"parent_id":  map[string]string{"type": "string", "description": "New parent department ID or name"},
					"sort_order": map[string]string{"type": "integer", "description": "New sort order"},
				},
			},
		},
		{
			Name:        utils.DeleteDepartment,
			Description: "Delete a department. Fails if it has child departments or associated workers.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": map[string]string{"type": "string", "description": "Department ID or name"},
				},
			},
		},
```

- [ ] **Step 2: Add cases to `beeCallTool`**

```go
	case utils.CreateDepartment:
		return s.toolCreateDepartment(args)
	case utils.UpdateDepartment:
		return s.toolUpdateDepartment(args)
	case utils.DeleteDepartment:
		return s.toolDeleteDepartment(args)
```

- [ ] **Step 3: Add handler implementations**

Add to `tools.go`:

```go
func (s *MCPServer) toolCreateDepartment(args json.RawMessage) (any, error) {
	var params struct {
		Name      string `json:"name"`
		ParentID  string `json:"parent_id"`
		SortOrder int    `json:"sort_order"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	d := model.Department{
		Name:      params.Name,
		SortOrder: params.SortOrder,
	}
	if params.ParentID != "" {
		parentID, err := s.resolveDepartmentID(params.ParentID)
		if err != nil {
			return nil, fmt.Errorf("parent: %w", err)
		}
		d.ParentID = &parentID
	}
	return s.departmentStore.Create(d)
}

func (s *MCPServer) toolUpdateDepartment(args json.RawMessage) (any, error) {
	var params struct {
		ID        string  `json:"id"`
		Name      *string `json:"name"`
		ParentID  *string `json:"parent_id"`
		SortOrder *int    `json:"sort_order"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	deptID, err := s.resolveDepartmentID(params.ID)
	if err != nil {
		return nil, err
	}
	d, err := s.departmentStore.GetByID(deptID)
	if err != nil {
		return nil, fmt.Errorf("get department: %w", err)
	}
	if params.Name != nil {
		d.Name = *params.Name
	}
	if params.SortOrder != nil {
		d.SortOrder = *params.SortOrder
	}
	if params.ParentID != nil {
		resolvedParentID, err := s.resolveDepartmentID(*params.ParentID)
		if err != nil {
			return nil, fmt.Errorf("parent: %w", err)
		}
		if err := s.departmentStore.CheckCircularReference(d.ID, resolvedParentID); err != nil {
			return nil, err
		}
		d.ParentID = &resolvedParentID
	}
	return s.departmentStore.Update(d)
}

func (s *MCPServer) toolDeleteDepartment(args json.RawMessage) (any, error) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	deptID, err := s.resolveDepartmentID(params.ID)
	if err != nil {
		return nil, err
	}
	if err := s.departmentStore.Delete(deptID); err != nil {
		return nil, err
	}
	return map[string]string{"status": "deleted"}, nil
}
```

- [ ] **Step 4: Write failing tests**

Add to `tools_test.go`:

```go
func TestCallTool_CreateDepartment(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	result, err := s.CallTool(context.Background(), "create_department",
		mustMarshal(t, map[string]any{"name": "Engineering"}))
	if err != nil {
		t.Fatalf("create_department: %v", err)
	}
	dept, ok := result.(model.Department)
	if !ok {
		t.Fatalf("expected model.Department, got %T", result)
	}
	if dept.ID == "" {
		t.Error("expected non-empty ID")
	}
	if dept.Name != "Engineering" {
		t.Errorf("expected name Engineering, got %s", dept.Name)
	}
}

func TestCallTool_CreateDepartment_WithParentByName(t *testing.T) {
	s, db := setupMCPServerWithSender(t)
	ds := store.NewDepartmentStore(db)
	parent, _ := ds.Create(model.Department{Name: "R&D"})

	result, err := s.CallTool(context.Background(), "create_department",
		mustMarshal(t, map[string]any{"name": "Frontend", "parent_id": "R&D"}))
	if err != nil {
		t.Fatalf("create_department with parent: %v", err)
	}
	child, ok := result.(model.Department)
	if !ok {
		t.Fatalf("expected model.Department, got %T", result)
	}
	if child.ParentID == nil || *child.ParentID != parent.ID {
		t.Errorf("expected ParentID %s, got %v", parent.ID, child.ParentID)
	}
}

func TestCallTool_UpdateDepartment_Name(t *testing.T) {
	s, db := setupMCPServerWithSender(t)
	ds := store.NewDepartmentStore(db)
	dept, _ := ds.Create(model.Department{Name: "OldName"})

	result, err := s.CallTool(context.Background(), "update_department",
		mustMarshal(t, map[string]any{"id": dept.ID, "name": "NewName"}))
	if err != nil {
		t.Fatalf("update_department: %v", err)
	}
	updated, ok := result.(model.Department)
	if !ok {
		t.Fatalf("expected model.Department, got %T", result)
	}
	if updated.Name != "NewName" {
		t.Errorf("expected name NewName, got %s", updated.Name)
	}
}

func TestCallTool_DeleteDepartment(t *testing.T) {
	s, db := setupMCPServerWithSender(t)
	ds := store.NewDepartmentStore(db)
	dept, _ := ds.Create(model.Department{Name: "ToDelete"})

	_, err := s.CallTool(context.Background(), "delete_department",
		mustMarshal(t, map[string]any{"id": dept.ID}))
	if err != nil {
		t.Fatalf("delete_department: %v", err)
	}

	// Verify it's gone
	_, err = ds.GetByID(dept.ID)
	if err == nil {
		t.Error("expected error fetching deleted department, got nil")
	}
}

func TestCallTool_DeleteDepartment_FailsWithChildren(t *testing.T) {
	s, db := setupMCPServerWithSender(t)
	ds := store.NewDepartmentStore(db)
	parent, _ := ds.Create(model.Department{Name: "Parent"})
	_, _ = ds.Create(model.Department{Name: "Child", ParentID: &parent.ID})

	_, err := s.CallTool(context.Background(), "delete_department",
		mustMarshal(t, map[string]any{"id": parent.ID}))
	if err == nil {
		t.Error("expected error deleting department with children, got nil")
	}
}
```

- [ ] **Step 5: Run the tests**

```bash
go test ./internal/ai/mcp/... -run "TestCallTool_(Create|Update|Delete)Department" -v 2>&1
```
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ai/mcp/tools.go internal/ai/mcp/tools_test.go
git commit -m "feat: add create_department, update_department, delete_department MCP tools"
```

---

## Task 6: Extend `list_workers` with department filtering

**Files:**
- Modify: `internal/ai/mcp/tools.go`
- Modify: `internal/ai/mcp/tools_test.go`

- [ ] **Step 1: Update `list_workers` schema in `beeToolSchemas()`**

Replace the current `list_workers` schema (which has empty properties) with:

```go
		{
			Name:        utils.ListWorkers,
			Description: "List all workers, optionally filtered by department",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"department_id": map[string]string{"type": "string", "description": "Filter by department ID or name"},
					"recursive":     map[string]any{"type": "boolean", "description": "Include workers in child departments (default: true)", "default": true},
				},
			},
		},
```

- [ ] **Step 2: Update `toolListWorkers` handler**

Replace the current `toolListWorkers` (which ignores args) with:

```go
func (s *MCPServer) toolListWorkers(args json.RawMessage) (any, error) {
	var params struct {
		DepartmentID string `json:"department_id"`
		Recursive    *bool  `json:"recursive"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}

	var workers []model.Worker
	var err error

	if params.DepartmentID != "" {
		deptID, resolveErr := s.resolveDepartmentID(params.DepartmentID)
		if resolveErr != nil {
			return nil, resolveErr
		}
		recursive := params.Recursive == nil || *params.Recursive
		if recursive {
			workers, err = s.listWorkersRecursive(deptID)
		} else {
			workers, err = s.workerStore.GetByDepartmentID(deptID)
		}
	} else {
		workers, err = s.workerStore.List()
	}

	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	if workers == nil {
		workers = []model.Worker{}
	}
	return workers, nil
}
```

- [ ] **Step 3: Write failing tests**

Add to `tools_test.go`:

```go
func TestCallTool_ListWorkers_FilterByDepartment(t *testing.T) {
	s, db := setupMCPServerWithSender(t)
	ds := store.NewDepartmentStore(db)
	ws := store.NewWorkerStore(db)

	dept, _ := ds.Create(model.Department{Name: "Engineering"})
	other, _ := ds.Create(model.Department{Name: "Marketing"})

	w1, _ := ws.Create(model.Worker{Name: "Alice"})
	w2, _ := ws.Create(model.Worker{Name: "Bob"})
	_ = ds.SetWorkerDepartments(w1.ID, []string{dept.ID})
	_ = ds.SetWorkerDepartments(w2.ID, []string{other.ID})

	result, err := s.CallTool(context.Background(), "list_workers",
		mustMarshal(t, map[string]any{"department_id": dept.ID}))
	if err != nil {
		t.Fatalf("list_workers with dept filter: %v", err)
	}
	workers, ok := result.([]model.Worker)
	if !ok {
		t.Fatalf("expected []model.Worker, got %T", result)
	}
	if len(workers) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(workers))
	}
	if workers[0].Name != "Alice" {
		t.Errorf("expected Alice, got %s", workers[0].Name)
	}
}

func TestCallTool_ListWorkers_FilterByDepartment_Recursive(t *testing.T) {
	s, db := setupMCPServerWithSender(t)
	ds := store.NewDepartmentStore(db)
	ws := store.NewWorkerStore(db)

	parent, _ := ds.Create(model.Department{Name: "R&D"})
	child, _ := ds.Create(model.Department{Name: "Frontend", ParentID: &parent.ID})

	w1, _ := ws.Create(model.Worker{Name: "Alice"})
	w2, _ := ws.Create(model.Worker{Name: "Bob"})
	_ = ds.SetWorkerDepartments(w1.ID, []string{parent.ID})
	_ = ds.SetWorkerDepartments(w2.ID, []string{child.ID})

	// recursive (default): should return both
	result, err := s.CallTool(context.Background(), "list_workers",
		mustMarshal(t, map[string]any{"department_id": parent.ID}))
	if err != nil {
		t.Fatalf("list_workers recursive: %v", err)
	}
	workers := result.([]model.Worker)
	if len(workers) != 2 {
		t.Errorf("expected 2 workers (recursive), got %d", len(workers))
	}

	// non-recursive: should return only Alice
	result2, err := s.CallTool(context.Background(), "list_workers",
		mustMarshal(t, map[string]any{"department_id": parent.ID, "recursive": false}))
	if err != nil {
		t.Fatalf("list_workers non-recursive: %v", err)
	}
	workers2 := result2.([]model.Worker)
	if len(workers2) != 1 {
		t.Errorf("expected 1 worker (non-recursive), got %d", len(workers2))
	}
	if workers2[0].Name != "Alice" {
		t.Errorf("expected Alice, got %s", workers2[0].Name)
	}
}
```

- [ ] **Step 4: Run the tests**

```bash
go test ./internal/ai/mcp/... -run "TestCallTool_ListWorkers_Filter" -v 2>&1
```
Expected: all PASS

- [ ] **Step 5: Confirm existing `ListWorkers` tests still pass**

```bash
go test ./internal/ai/mcp/... -run "TestCallTool_ListWorkers" -v 2>&1
```
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ai/mcp/tools.go internal/ai/mcp/tools_test.go
git commit -m "feat: extend list_workers with department_id filter and recursive support"
```

---

## Task 7: Extend `create_worker` and `update_worker` with `department_ids`

**Files:**
- Modify: `internal/ai/mcp/tools.go`
- Modify: `internal/ai/mcp/tools_test.go`

- [ ] **Step 1: Update `create_worker` schema**

In `beeToolSchemas()`, add `department_ids` to the `create_worker` properties:

```go
		{
			Name:        utils.CreateWorker,
			Description: "Create a new worker",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]any{
					"name":           map[string]string{"type": "string", "description": "Worker name"},
					"description":    map[string]string{"type": "string", "description": "Worker description"},
					"memory":         map[string]string{"type": "string", "description": "Worker memory content"},
					"work_dir":       map[string]string{"type": "string", "description": "Working directory path (optional, auto-assigned if empty)"},
					"department_ids": map[string]string{"type": "string", "description": "Comma-separated department IDs or names to associate the worker with"},
				},
			},
		},
```

- [ ] **Step 2: Update `update_worker` schema**

Add `department_ids` to the `update_worker` properties:

```go
		{
			Name:        utils.UpdateWorker,
			Description: "Update a worker's name, description, or memory (patch semantics: omitted fields unchanged)",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"worker_id"},
				"properties": map[string]any{
					"worker_id":      map[string]string{"type": "string", "description": "Worker ID"},
					"name":           map[string]string{"type": "string", "description": "New name"},
					"description":    map[string]string{"type": "string", "description": "New description"},
					"memory":         map[string]string{"type": "string", "description": "New memory content"},
					"department_ids": map[string]string{"type": "string", "description": "Comma-separated department IDs or names; replaces all existing associations. Empty string clears all."},
				},
			},
		},
```

- [ ] **Step 3: Update `toolCreateWorker` handler**

Replace the current `toolCreateWorker` with:

```go
func (s *MCPServer) toolCreateWorker(args json.RawMessage) (any, error) {
	var params struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		Memory        string `json:"memory"`
		WorkDir       string `json:"work_dir"`
		DepartmentIDs string `json:"department_ids"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	w, err := s.manager.CreateWorker(params.Name, params.Description, params.Memory, params.WorkDir)
	if err != nil {
		return nil, err
	}
	if params.DepartmentIDs != "" {
		deptIDs, err := s.resolveDepartmentIDs(splitAndTrim(params.DepartmentIDs))
		if err != nil {
			return nil, fmt.Errorf("set departments: %w", err)
		}
		if err := s.departmentStore.SetWorkerDepartments(w.ID, deptIDs); err != nil {
			return nil, fmt.Errorf("set worker departments: %w", err)
		}
	}
	return w, nil
}
```

- [ ] **Step 4: Update `toolUpdateWorker` handler**

Replace the current `toolUpdateWorker` with:

```go
func (s *MCPServer) toolUpdateWorker(args json.RawMessage) (any, error) {
	var params struct {
		WorkerID      string  `json:"worker_id"`
		Name          *string `json:"name"`
		Description   *string `json:"description"`
		Memory        *string `json:"memory"`
		DepartmentIDs *string `json:"department_ids"`
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
	if params.Name != nil {
		w.Name = *params.Name
	}
	if params.Description != nil {
		w.Description = *params.Description
	}
	if params.Memory != nil {
		w.Memory = *params.Memory
	}
	w, err = s.workerStore.Update(w)
	if err != nil {
		return nil, err
	}
	if params.DepartmentIDs != nil {
		var deptIDs []string
		if *params.DepartmentIDs != "" {
			deptIDs, err = s.resolveDepartmentIDs(splitAndTrim(*params.DepartmentIDs))
			if err != nil {
				return nil, fmt.Errorf("set departments: %w", err)
			}
		}
		if err := s.departmentStore.SetWorkerDepartments(w.ID, deptIDs); err != nil {
			return nil, fmt.Errorf("set worker departments: %w", err)
		}
	}
	return w, nil
}
```

- [ ] **Step 5: Write failing tests**

Add to `tools_test.go`:

```go
func TestCallTool_CreateWorker_WithDepartment(t *testing.T) {
	s, db := setupMCPServerWithSender(t)
	ds := store.NewDepartmentStore(db)
	dept, _ := ds.Create(model.Department{Name: "Engineering"})

	result, err := s.CallTool(context.Background(), "create_worker",
		mustMarshal(t, map[string]any{"name": "Alice", "department_ids": dept.ID}))
	if err != nil {
		t.Fatalf("create_worker with dept: %v", err)
	}
	w, ok := result.(model.Worker)
	if !ok {
		t.Fatalf("expected model.Worker, got %T", result)
	}

	depts, err := ds.GetWorkerDepartments(w.ID)
	if err != nil {
		t.Fatalf("GetWorkerDepartments: %v", err)
	}
	if len(depts) != 1 || depts[0].ID != dept.ID {
		t.Errorf("expected worker in Engineering dept, got %v", depts)
	}
}

func TestCallTool_UpdateWorker_SetDepartments(t *testing.T) {
	s, db := setupMCPServerWithSender(t)
	ds := store.NewDepartmentStore(db)
	ws := store.NewWorkerStore(db)

	dept1, _ := ds.Create(model.Department{Name: "Engineering"})
	dept2, _ := ds.Create(model.Department{Name: "Design"})
	w, _ := ws.Create(model.Worker{Name: "Alice"})
	_ = ds.SetWorkerDepartments(w.ID, []string{dept1.ID})

	// Replace department1 with department2
	_, err := s.CallTool(context.Background(), "update_worker",
		mustMarshal(t, map[string]any{"worker_id": w.ID, "department_ids": dept2.Name}))
	if err != nil {
		t.Fatalf("update_worker set dept: %v", err)
	}

	depts, _ := ds.GetWorkerDepartments(w.ID)
	if len(depts) != 1 || depts[0].ID != dept2.ID {
		t.Errorf("expected worker in Design dept only, got %v", depts)
	}
}

func TestCallTool_UpdateWorker_ClearDepartments(t *testing.T) {
	s, db := setupMCPServerWithSender(t)
	ds := store.NewDepartmentStore(db)
	ws := store.NewWorkerStore(db)

	dept, _ := ds.Create(model.Department{Name: "Engineering"})
	w, _ := ws.Create(model.Worker{Name: "Alice"})
	_ = ds.SetWorkerDepartments(w.ID, []string{dept.ID})

	_, err := s.CallTool(context.Background(), "update_worker",
		mustMarshal(t, map[string]any{"worker_id": w.ID, "department_ids": ""}))
	if err != nil {
		t.Fatalf("update_worker clear depts: %v", err)
	}

	depts, _ := ds.GetWorkerDepartments(w.ID)
	if len(depts) != 0 {
		t.Errorf("expected 0 departments after clear, got %d", len(depts))
	}
}
```

- [ ] **Step 6: Run the new tests**

```bash
go test ./internal/ai/mcp/... -run "TestCallTool_(CreateWorker_With|UpdateWorker_Set|UpdateWorker_Clear)" -v 2>&1
```
Expected: all PASS

- [ ] **Step 7: Run full test suite to confirm nothing regressed**

```bash
go test ./internal/ai/mcp/... -v -count=1 2>&1 | tail -30
```
Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add internal/ai/mcp/tools.go internal/ai/mcp/tools_test.go
git commit -m "feat: extend create_worker and update_worker with department_ids support"
```

---

## Task 8: Create `cmd/openbee/ctl_department.go`

**Files:**
- Create: `cmd/openbee/ctl_department.go`

- [ ] **Step 1: Create the file**

Create `cmd/openbee/ctl_department.go` with this content:

```go
package main

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

var ctlDepartmentCmd = &cobra.Command{Use: "department", Short: "Manage departments"}

var ctlDepartmentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all departments (tree structure)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.ListDepartments, nil)
	},
}

var ctlDepartmentGetCmd = &cobra.Command{
	Use:   "get <id|name>",
	Short: "Get a department by ID or name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.GetDepartment, map[string]any{"id": args[0]})
	},
}

var (
	departmentCreateName      string
	departmentCreateParent    string
	departmentCreateSortOrder int
)

var ctlDepartmentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new department",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"name": departmentCreateName}
		if departmentCreateParent != "" {
			a["parent_id"] = departmentCreateParent
		}
		if cmd.Flags().Changed("sort-order") {
			a["sort_order"] = departmentCreateSortOrder
		}
		return ctlRun(utils.CreateDepartment, a)
	},
}

var (
	departmentUpdateName      string
	departmentUpdateParent    string
	departmentUpdateSortOrder int
)

var ctlDepartmentUpdateCmd = &cobra.Command{
	Use:   "update <id|name>",
	Short: "Update a department (patch: omitted fields unchanged)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"id": args[0]}
		if cmd.Flags().Changed("name") {
			a["name"] = departmentUpdateName
		}
		if cmd.Flags().Changed("parent") {
			a["parent_id"] = departmentUpdateParent
		}
		if cmd.Flags().Changed("sort-order") {
			a["sort_order"] = departmentUpdateSortOrder
		}
		return ctlRun(utils.UpdateDepartment, a)
	},
}

var ctlDepartmentDeleteCmd = &cobra.Command{
	Use:   "delete <id|name>",
	Short: "Delete a department",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.DeleteDepartment, map[string]any{"id": args[0]})
	},
}

func init() {
	ctlDepartmentCreateCmd.Flags().StringVarP(&departmentCreateName, "name", "n", "", "Department name (required)")
	ctlDepartmentCreateCmd.MarkFlagRequired("name")
	ctlDepartmentCreateCmd.Flags().StringVar(&departmentCreateParent, "parent", "", "Parent department ID or name")
	ctlDepartmentCreateCmd.Flags().IntVar(&departmentCreateSortOrder, "sort-order", 0, "Display sort order")

	ctlDepartmentUpdateCmd.Flags().StringVar(&departmentUpdateName, "name", "", "New name")
	ctlDepartmentUpdateCmd.Flags().StringVar(&departmentUpdateParent, "parent", "", "New parent department ID or name")
	ctlDepartmentUpdateCmd.Flags().IntVar(&departmentUpdateSortOrder, "sort-order", 0, "New sort order")

	ctlDepartmentCmd.AddCommand(
		ctlDepartmentListCmd,
		ctlDepartmentGetCmd,
		ctlDepartmentCreateCmd,
		ctlDepartmentUpdateCmd,
		ctlDepartmentDeleteCmd,
	)
	ctlCmd.AddCommand(ctlDepartmentCmd)
}
```

- [ ] **Step 2: Build to confirm no compile errors**

```bash
go build ./cmd/openbee/...
```
Expected: no output

- [ ] **Step 3: Verify help output**

```bash
go run ./cmd/openbee ctl department --help
```
Expected: shows `list`, `get`, `create`, `update`, `delete` subcommands

- [ ] **Step 4: Commit**

```bash
git add cmd/openbee/ctl_department.go
git commit -m "feat: add openbee ctl department CLI subcommands"
```

---

## Task 9: Extend `ctl worker` with `--department` and `--no-recursive`

**Files:**
- Modify: `cmd/openbee/ctl_worker.go`

- [ ] **Step 1: Add package-level variables for new flags**

After the existing `var workerDeleteWorkDir bool` line, add:

```go
var (
	workerListDepartment  string
	workerListNoRecursive bool
	workerCreateDepartment string
	workerUpdateDepartment string
)
```

- [ ] **Step 2: Update `ctlWorkerListCmd` to support department filtering**

Replace the current `ctlWorkerListCmd`:

```go
var ctlWorkerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all workers",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{}
		if workerListDepartment != "" {
			a["department_id"] = workerListDepartment
			if workerListNoRecursive {
				a["recursive"] = false
			}
		}
		return ctlRun(utils.ListWorkers, a)
	},
}
```

- [ ] **Step 3: Update `ctlWorkerCreateCmd` to support `--department`**

Replace the current `ctlWorkerCreateCmd`:

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
		return ctlRun(utils.CreateWorker, a)
	},
}
```

- [ ] **Step 4: Update `ctlWorkerUpdateCmd` to support `--department`**

Replace the current `ctlWorkerUpdateCmd`:

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
		return ctlRun(utils.UpdateWorker, a)
	},
}
```

- [ ] **Step 5: Register the new flags in `init()`**

In the `init()` function, add after the existing flag registrations:

```go
	// list flags
	ctlWorkerListCmd.Flags().StringVar(&workerListDepartment, "department", "", "Filter by department ID or name")
	ctlWorkerListCmd.Flags().BoolVar(&workerListNoRecursive, "no-recursive", false, "Only return workers directly in the department (not in child departments)")

	// create flag
	ctlWorkerCreateCmd.Flags().StringVar(&workerCreateDepartment, "department", "", "Department ID or name (comma-separated for multiple)")

	// update flag
	ctlWorkerUpdateCmd.Flags().StringVar(&workerUpdateDepartment, "department", "", "Department ID or name (comma-separated); replaces all associations. Pass empty string to clear.")
```

- [ ] **Step 6: Build and verify help**

```bash
go build ./cmd/openbee/... && go run ./cmd/openbee ctl worker list --help
```
Expected: shows `--department` and `--no-recursive` flags

```bash
go run ./cmd/openbee ctl worker create --help
```
Expected: shows `--department` flag

```bash
go run ./cmd/openbee ctl worker update --help
```
Expected: shows `--department` flag

- [ ] **Step 7: Run full test suite one final time**

```bash
go test ./... 2>&1 | tail -30
```
Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add cmd/openbee/ctl_worker.go
git commit -m "feat: add --department and --no-recursive flags to ctl worker commands"
```

---

## Self-Review Checklist

### Spec coverage

| Spec requirement | Task |
|-----------------|------|
| `department list` returns tree | Task 4 (`toolListDepartments`) |
| `department get <id\|name>` | Task 4 (`toolGetDepartment`) |
| `department create --name --parent --sort-order` | Task 5 + Task 8 CLI |
| `department update <id\|name>` patch semantics | Task 5 + Task 8 CLI |
| `department delete` fails with children/workers | Task 5 (`departmentStore.Delete` already handles) |
| `worker list --department` with recursive default | Task 6 + Task 9 |
| `worker list --no-recursive` | Task 6 + Task 9 |
| `worker create --department` comma-separated | Task 7 + Task 9 |
| `worker update --department` replaces all | Task 7 + Task 9 |
| Name auto-detection (ID first, then name) | Task 3 (`resolveDepartmentID`) |
| Ambiguous name error with ancestor paths | Task 3 (`departmentAncestorPath`) |
| `departmentStore` injected via constructor | Task 2 |

All spec requirements covered. ✓
