# Group Collaboration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the Group collaboration feature: a `Group` is a peer-to-Worker Agent that receives a root task, splits it into sub-tasks for member Workers, monitors progress via event-driven session resume, and is the only voice that talks back to the user.

**Architecture:** Group reuses Worker infrastructure (engine adapters, dispatcher, sessions, executions). Sub-tasks are stored in the existing `bee_tasks` table with new `parent_task_id`/`root_task_id`/`agent_kind` columns and a new `waiting_subtasks` status. The Group agent calls four new `openbee ctl task` sub-commands (`dispatch-subtask`, `subtasks`, `suspend`, `mark-success`, `mark-failed`); after `suspend`, dispatcher resumes the Group whenever a sub-task transitions to a terminal state by injecting an `<subtask_event>` snapshot built from the live tasks table.

**Tech Stack:** Go, SQLite (via `database/sql`), Gin (REST), Cobra (CLI), zap (logging), uuid. Test framework: stdlib `testing` + real in-memory SQLite + the existing fakes pattern (`fakeExecutionManager`, `fakeStore`, `fakeSessionStore`).

**Spec:** `docs/superpowers/specs/2026-04-27-group-collaboration-design.md` (commit `90e6ad0`).

---

## File Map

### New files

| Path | Responsibility |
|---|---|
| `internal/infra/model/group.go` | `Group`, `WorkerGroup`, `GroupBrief` types |
| `internal/infra/store/group_store.go` | DDL handled in migrations; CRUD + membership ops |
| `internal/infra/store/group_store_test.go` | Unit tests for store |
| `internal/domain/group/manager.go` | `Manager`: CRUD, member ops, persona prep, name collision check |
| `internal/domain/group/manager_test.go` | Unit tests for manager |
| `internal/domain/group/persona.go` | `BuildPersona` builder consumed by dispatcher |
| `internal/domain/group/persona_test.go` | Unit tests for persona builder |
| `cmd/openbee/ctl_group.go` | `openbee ctl group ...` CLI tree |
| `internal/api/group_handler.go` | Gin handlers for group CRUD + member ops |
| `internal/api/group_handler_test.go` | HTTP tests |
| `internal/api/subtask_handler.go` | Gin handlers for `dispatch-subtask`, `subtasks`, `suspend`, `mark-success/failed` |
| `internal/api/subtask_handler_test.go` | HTTP tests |
| `internal/domain/task/recovery.go` | `RecoverGroupTasks` startup hook |
| `internal/domain/task/recovery_test.go` | Tests |
| `internal/domain/task/e2e_group_test.go` | End-to-end test using fake engine |

### Modified files

| Path | Why |
|---|---|
| `internal/infra/store/db.go` | Append migrations 32–38 (groups, worker_groups, tasks columns, status enum, indexes) |
| `internal/infra/store/task_store.go` | Add `parent_task_id`/`root_task_id`/`agent_kind` to scan/insert; add `ListByRoot`, `MarkWaitingSubtasks`, `GetParent` methods |
| `internal/infra/store/task_store_test.go` | Tests for new methods |
| `internal/infra/model/task.go` | Add `TaskStatusWaitingSubtasks`, `AgentKind*` consts and new fields |
| `internal/domain/task/dispatcher.go` | Branch on `agent_kind`; new sub-task event resume logic; new cancellation cascade |
| `internal/domain/task/dispatcher_test.go` (or `dispatcher_internal_test.go`) | New tests covering Group path |
| `internal/domain/bee/feeder.go` | `tryDirectDispatch` looks up by group name as well |
| `internal/domain/bee/feeder_test.go` | Tests for group-name dispatch |
| `cmd/openbee/ctl_task.go` | Register five new sub-commands wired to `internal/infra/utils` actions |
| `internal/infra/utils/actions.go` (or wherever `utils.ListWorkers` etc. live) | Add `DispatchSubtask`, `ListSubtasks`, `SuspendTask`, `MarkTaskSuccess`, `MarkTaskFailed`, plus `Group*` actions |
| `cmd/openbee/ctl_message.go` (server-side handler — likely `internal/api/message_handler.go`) | Reroute `message send` when current task has `parent_task_id` |
| `internal/routes/api.go` | Wire new handlers into the gin router |
| `internal/app/app.go` | Construct `GroupStore`, `group.Manager`, register dispatcher's `RecoverGroupTasks` hook on startup |

### Decomposition notes

- Each store / handler / manager file is responsible for one entity. No cross-entity helpers.
- `internal/domain/group/persona.go` is split out of `manager.go` because dispatcher imports it directly during runtime injection — keeping it in its own file with a stable public surface lets dispatcher depend on a small interface.
- New CLI sub-commands intentionally pass through the existing `ctlRun(utils.<Action>, args)` pattern so server-side action wiring is the single source of truth — the CLI files stay tiny.

---

## Conventions used throughout the plan

- **TDD ordering**: write failing test → run it (assert it fails) → implement → run again (assert pass) → commit.
- **Commands**: every test runs from repo root via `go test ./internal/...` filtered to the relevant package.
- **Commit messages**: `<scope>: <imperative summary>`, matching repo style (e.g. `feat(group): add group store`).
- **`bee_` prefix**: every new table starts with `bee_`.
- **`_test.go` files**: same package as the file they test; use the `setup<Thing>TestDB` helper pattern that already exists in this repo.

---

# Phase 1 — Data Layer

## Task 1: Database migrations

**Files:**
- Modify: `internal/infra/store/db.go` (append to `migrations` slice; current max version is ~31, use 32–38)
- Test: `internal/infra/store/db_test.go` (existing migration test will run; no new test file needed)

- [ ] **Step 1.1: Append migrations 32–38**

Open `internal/infra/store/db.go`, locate the end of the `migrations` slice, and append:

```go
{
    version: 32,
    name:    "create_table_bee_groups",
    sql: `CREATE TABLE IF NOT EXISTS bee_groups (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    constraints       TEXT NOT NULL DEFAULT '',
    work_dir          TEXT NOT NULL,
    engine            TEXT NOT NULL DEFAULT '',
    engine_args       TEXT NOT NULL DEFAULT '{}',
    status            TEXT NOT NULL DEFAULT 'idle'
                          CHECK(status IN ('idle','working','error')),
    permission_scopes TEXT NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
)`,
},
{
    version: 33,
    name:    "create_index_groups_name_lower",
    sql:     `CREATE INDEX IF NOT EXISTS idx_groups_name_lower ON bee_groups (LOWER(name))`,
},
{
    version: 34,
    name:    "create_table_bee_worker_groups",
    sql: `CREATE TABLE IF NOT EXISTS bee_worker_groups (
    worker_id  TEXT NOT NULL REFERENCES bee_workers(id),
    group_id   TEXT NOT NULL REFERENCES bee_groups(id),
    role       TEXT NOT NULL DEFAULT 'member',
    created_at INTEGER NOT NULL,
    PRIMARY KEY (worker_id, group_id)
)`,
},
{
    version: 35,
    name:    "create_indexes_worker_groups",
    sql: `CREATE INDEX IF NOT EXISTS idx_worker_groups_worker ON bee_worker_groups(worker_id);
CREATE INDEX IF NOT EXISTS idx_worker_groups_group  ON bee_worker_groups(group_id);`,
},
{
    version: 36,
    name:    "add_parent_root_agent_kind_to_tasks",
    sql: `ALTER TABLE bee_tasks ADD COLUMN parent_task_id TEXT NOT NULL DEFAULT '';
ALTER TABLE bee_tasks ADD COLUMN root_task_id   TEXT NOT NULL DEFAULT '';
ALTER TABLE bee_tasks ADD COLUMN agent_kind     TEXT NOT NULL DEFAULT 'worker'
    CHECK(agent_kind IN ('worker','group'));`,
},
{
    version: 37,
    name:    "backfill_root_task_id_self_reference",
    sql:     `UPDATE bee_tasks SET root_task_id = id WHERE root_task_id = ''`,
},
{
    version: 38,
    name:    "create_index_tasks_parent_root",
    sql: `CREATE INDEX IF NOT EXISTS idx_tasks_parent ON bee_tasks(parent_task_id) WHERE parent_task_id != '';
CREATE INDEX IF NOT EXISTS idx_tasks_root ON bee_tasks(root_task_id);`,
},
```

- [ ] **Step 1.2: Verify the existing migration test still passes**

Run:
```bash
go test ./internal/infra/store/ -run TestInitDB -v
```
Expected: PASS. The existing test already verifies that the migration list applies cleanly to a fresh DB.

- [ ] **Step 1.3: Add a migration-idempotency assertion**

If `db_test.go` doesn't already cover idempotency, add to `internal/infra/store/db_test.go`:

```go
func TestInitDB_Idempotent(t *testing.T) {
    path := t.TempDir() + "/test.db"
    db1, err := InitDB(path)
    if err != nil {
        t.Fatalf("first InitDB: %v", err)
    }
    db1.Close()
    db2, err := InitDB(path)
    if err != nil {
        t.Fatalf("second InitDB (idempotent): %v", err)
    }
    db2.Close()
}
```

If a similar test already exists, skip.

Run:
```bash
go test ./internal/infra/store/ -run TestInitDB -v
```
Expected: PASS.

- [ ] **Step 1.4: Commit**

```bash
git add internal/infra/store/db.go internal/infra/store/db_test.go
git commit -m "feat(store): add migrations 32-38 for groups and task tree"
```

---

## Task 2: Group model

**Files:**
- Create: `internal/infra/model/group.go`

- [ ] **Step 2.1: Create the model file**

```go
package model

type GroupStatus = WorkerStatus // reuse idle/working/error vocabulary

// Group is an Agent that coordinates Worker members on a root task.
type Group struct {
    ID               string      `json:"id" db:"id"`
    Name             string      `json:"name" db:"name"`
    Description      string      `json:"description" db:"description"`
    Constraints      string      `json:"constraints" db:"constraints"`
    WorkDir          string      `json:"work_dir" db:"work_dir"`
    Engine           string      `json:"engine" db:"engine"`
    EngineArgs       string      `json:"engine_args" db:"engine_args"`
    Status           GroupStatus `json:"status" db:"status"`
    PermissionScopes string      `json:"permission_scopes" db:"permission_scopes"`
    CreatedAt        int64       `json:"created_at" db:"created_at"`
    UpdatedAt        int64       `json:"updated_at" db:"updated_at"`
}

// WorkerGroup is the many-to-many membership row.
type WorkerGroup struct {
    WorkerID  string `json:"worker_id" db:"worker_id"`
    GroupID   string `json:"group_id" db:"group_id"`
    Role      string `json:"role" db:"role"` // reserved for future use; default "member"
    CreatedAt int64  `json:"created_at" db:"created_at"`
}

// GroupBrief is a lightweight Group summary used in list responses or membership reports.
type GroupBrief struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

// MemberBrief is a lightweight Worker summary used inside a group response.
type MemberBrief struct {
    ID          string `json:"id"`
    Name        string `json:"name"`
    Description string `json:"description"`
}

// GroupWithMembers extends Group with its current member roster.
type GroupWithMembers struct {
    Group
    Members []MemberBrief `json:"members"`
}
```

- [ ] **Step 2.2: Verify it compiles**

```bash
go build ./internal/infra/model/...
```
Expected: no output (success).

- [ ] **Step 2.3: Add task model constants**

Edit `internal/infra/model/task.go` and add:

```go
const (
    TaskStatusWaitingSubtasks = "waiting_subtasks"

    AgentKindWorker = "worker"
    AgentKindGroup  = "group"
)
```

Also add the new fields to the `Task` struct, immediately after `ExecutionID`:

```go
ParentTaskID string
RootTaskID   string
AgentKind    string
```

Update the `TaskStatusActive` const to include the new state:
```go
TaskStatusActive = TaskStatusPending + "," + TaskStatusRunning + "," + TaskStatusWaitingSubtasks
```

- [ ] **Step 2.4: Verify it compiles**

```bash
go build ./internal/infra/model/...
```
Expected: no output (success). Other packages will not build until Task 5 updates `task_store.go` — that's expected.

- [ ] **Step 2.5: Commit**

```bash
git add internal/infra/model/group.go internal/infra/model/task.go
git commit -m "feat(model): add Group, WorkerGroup, and task tree fields"
```

---

## Task 3: GroupStore — CRUD

**Files:**
- Create: `internal/infra/store/group_store.go`
- Create: `internal/infra/store/group_store_test.go`

- [ ] **Step 3.1: Write failing tests**

Create `internal/infra/store/group_store_test.go`:

```go
package store

import (
    "testing"

    "github.com/theopenbee/openbee/internal/infra/model"
)

func setupGroupTestDB(t *testing.T) (*GroupStore, *WorkerStore) {
    t.Helper()
    db, err := InitDB(t.TempDir() + "/test.db")
    if err != nil {
        t.Fatalf("InitDB: %v", err)
    }
    t.Cleanup(func() { db.Close() })
    return NewGroupStore(db), NewWorkerStore(db)
}

func TestGroupStore_Create(t *testing.T) {
    gs, _ := setupGroupTestDB(t)
    g, err := gs.Create(model.Group{Name: "data-team", WorkDir: "/tmp/g1"})
    if err != nil {
        t.Fatalf("Create: %v", err)
    }
    if g.ID == "" {
        t.Error("expected non-empty ID")
    }
    if g.Status != model.WorkerStatusIdle {
        t.Errorf("expected idle, got %s", g.Status)
    }
    if g.EngineArgs != "{}" {
        t.Errorf("expected default engine_args '{}', got %q", g.EngineArgs)
    }
}

func TestGroupStore_GetByID(t *testing.T) {
    gs, _ := setupGroupTestDB(t)
    g, _ := gs.Create(model.Group{Name: "ops", WorkDir: "/tmp/g2"})
    got, err := gs.GetByID(g.ID)
    if err != nil {
        t.Fatalf("GetByID: %v", err)
    }
    if got.Name != "ops" {
        t.Errorf("expected ops, got %s", got.Name)
    }
}

func TestGroupStore_GetByName(t *testing.T) {
    gs, _ := setupGroupTestDB(t)
    _, _ = gs.Create(model.Group{Name: "Alpha", WorkDir: "/tmp/g3"})
    got, err := gs.GetByName("alpha") // case-insensitive
    if err != nil {
        t.Fatalf("GetByName: %v", err)
    }
    if got.Name != "Alpha" {
        t.Errorf("expected Alpha, got %s", got.Name)
    }
}

func TestGroupStore_ExistsByName(t *testing.T) {
    gs, _ := setupGroupTestDB(t)
    g, _ := gs.Create(model.Group{Name: "Beta", WorkDir: "/tmp/g4"})
    yes, err := gs.ExistsByName("BETA", "")
    if err != nil || !yes {
        t.Fatalf("ExistsByName(BETA, ''): got (%v, %v), want (true, nil)", yes, err)
    }
    no, _ := gs.ExistsByName("BETA", g.ID) // exclude self
    if no {
        t.Errorf("ExistsByName(BETA, self): expected false")
    }
}

func TestGroupStore_List(t *testing.T) {
    gs, _ := setupGroupTestDB(t)
    _, _ = gs.Create(model.Group{Name: "g1", WorkDir: "/tmp/g1"})
    _, _ = gs.Create(model.Group{Name: "g2", WorkDir: "/tmp/g2"})
    list, err := gs.List()
    if err != nil {
        t.Fatalf("List: %v", err)
    }
    if len(list) != 2 {
        t.Errorf("expected 2, got %d", len(list))
    }
}

func TestGroupStore_Update(t *testing.T) {
    gs, _ := setupGroupTestDB(t)
    g, _ := gs.Create(model.Group{Name: "old", WorkDir: "/tmp/g1"})
    g.Name = "new"
    g.Description = "updated"
    out, err := gs.Update(g)
    if err != nil {
        t.Fatalf("Update: %v", err)
    }
    if out.Name != "new" || out.Description != "updated" {
        t.Errorf("update did not stick: %+v", out)
    }
}

func TestGroupStore_Delete(t *testing.T) {
    gs, _ := setupGroupTestDB(t)
    g, _ := gs.Create(model.Group{Name: "tmp", WorkDir: "/tmp/g1"})
    if err := gs.Delete(g.ID); err != nil {
        t.Fatalf("Delete: %v", err)
    }
    if _, err := gs.GetByID(g.ID); err == nil {
        t.Error("expected error after delete")
    }
}
```

- [ ] **Step 3.2: Run the tests — expect compile failure**

```bash
go test ./internal/infra/store/ -run TestGroupStore -v
```
Expected: FAIL with "undefined: NewGroupStore" or similar.

- [ ] **Step 3.3: Implement `group_store.go`**

Create `internal/infra/store/group_store.go`:

```go
package store

import (
    "database/sql"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/theopenbee/openbee/internal/infra/model"
)

type GroupStore struct {
    db *sql.DB
}

func NewGroupStore(db *sql.DB) *GroupStore {
    return &GroupStore{db: db}
}

const groupColumns = `id, name, description, constraints, work_dir, engine, engine_args, status, permission_scopes, created_at, updated_at`

func scanGroup(scanner interface{ Scan(...any) error }) (model.Group, error) {
    var g model.Group
    err := scanner.Scan(
        &g.ID, &g.Name, &g.Description, &g.Constraints,
        &g.WorkDir, &g.Engine, &g.EngineArgs, &g.Status,
        &g.PermissionScopes, &g.CreatedAt, &g.UpdatedAt,
    )
    return g, err
}

func scanGroups(rows *sql.Rows) ([]model.Group, error) {
    var out []model.Group
    for rows.Next() {
        g, err := scanGroup(rows)
        if err != nil {
            return nil, err
        }
        out = append(out, g)
    }
    return out, rows.Err()
}

func (s *GroupStore) Create(g model.Group) (model.Group, error) {
    if g.ID == "" {
        g.ID = uuid.New().String()
    }
    g.Status = model.WorkerStatusIdle
    g.CreatedAt = time.Now().UnixMilli()
    g.UpdatedAt = g.CreatedAt
    if g.EngineArgs == "" {
        g.EngineArgs = "{}"
    }
    _, err := s.db.Exec(
        `INSERT INTO bee_groups (`+groupColumns+`)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        g.ID, g.Name, g.Description, g.Constraints,
        g.WorkDir, g.Engine, g.EngineArgs, g.Status,
        g.PermissionScopes, g.CreatedAt, g.UpdatedAt,
    )
    if err != nil {
        return model.Group{}, fmt.Errorf("insert group: %w", err)
    }
    return g, nil
}

func (s *GroupStore) GetByID(id string) (model.Group, error) {
    row := s.db.QueryRow(`SELECT `+groupColumns+` FROM bee_groups WHERE id = ?`, id)
    g, err := scanGroup(row)
    if err != nil {
        return model.Group{}, fmt.Errorf("get group: %w", err)
    }
    return g, nil
}

func (s *GroupStore) GetByName(name string) (model.Group, error) {
    row := s.db.QueryRow(
        `SELECT `+groupColumns+` FROM bee_groups
         WHERE LOWER(name) = LOWER(?)
         ORDER BY created_at ASC, ROWID ASC LIMIT 1`,
        name,
    )
    g, err := scanGroup(row)
    if err != nil {
        return model.Group{}, fmt.Errorf("get group by name: %w", err)
    }
    return g, nil
}

func (s *GroupStore) ExistsByName(name, excludeID string) (bool, error) {
    var n int
    err := s.db.QueryRow(
        `SELECT EXISTS(SELECT 1 FROM bee_groups WHERE LOWER(name) = LOWER(?) AND id != ?)`,
        name, excludeID,
    ).Scan(&n)
    return n == 1, err
}

func (s *GroupStore) List() ([]model.Group, error) {
    rows, err := s.db.Query(`SELECT ` + groupColumns + ` FROM bee_groups ORDER BY created_at DESC`)
    if err != nil {
        return nil, fmt.Errorf("list groups: %w", err)
    }
    defer rows.Close()
    return scanGroups(rows)
}

func (s *GroupStore) Update(g model.Group) (model.Group, error) {
    g.UpdatedAt = time.Now().UnixMilli()
    _, err := s.db.Exec(
        `UPDATE bee_groups SET name=?, description=?, constraints=?, work_dir=?,
            engine=?, engine_args=?, status=?, permission_scopes=?, updated_at=?
         WHERE id=?`,
        g.Name, g.Description, g.Constraints, g.WorkDir,
        g.Engine, g.EngineArgs, g.Status, g.PermissionScopes, g.UpdatedAt,
        g.ID,
    )
    if err != nil {
        return model.Group{}, fmt.Errorf("update group: %w", err)
    }
    return g, nil
}

func (s *GroupStore) Delete(id string) error {
    _, err := s.db.Exec(`DELETE FROM bee_groups WHERE id = ?`, id)
    if err != nil {
        return fmt.Errorf("delete group: %w", err)
    }
    return nil
}
```

- [ ] **Step 3.4: Run the tests — expect PASS**

```bash
go test ./internal/infra/store/ -run TestGroupStore -v
```
Expected: PASS for all 7 tests.

- [ ] **Step 3.5: Commit**

```bash
git add internal/infra/store/group_store.go internal/infra/store/group_store_test.go
git commit -m "feat(store): add GroupStore CRUD"
```

---

## Task 4: GroupStore — membership ops

**Files:**
- Modify: `internal/infra/store/group_store.go` (append methods)
- Modify: `internal/infra/store/group_store_test.go` (append tests)

- [ ] **Step 4.1: Add failing tests**

Append to `internal/infra/store/group_store_test.go`:

```go
func TestGroupStore_AddMember(t *testing.T) {
    gs, ws := setupGroupTestDB(t)
    g, _ := gs.Create(model.Group{Name: "g", WorkDir: "/tmp/g"})
    w, _ := ws.Create(model.Worker{Name: "w", WorkDir: "/tmp/w"})

    if err := gs.AddMember(g.ID, w.ID, "member"); err != nil {
        t.Fatalf("AddMember: %v", err)
    }
    members, err := gs.ListMembers(g.ID)
    if err != nil {
        t.Fatalf("ListMembers: %v", err)
    }
    if len(members) != 1 || members[0].ID != w.ID {
        t.Errorf("expected 1 member with id %s, got %+v", w.ID, members)
    }
}

func TestGroupStore_AddMember_Idempotent(t *testing.T) {
    gs, ws := setupGroupTestDB(t)
    g, _ := gs.Create(model.Group{Name: "g", WorkDir: "/tmp/g"})
    w, _ := ws.Create(model.Worker{Name: "w", WorkDir: "/tmp/w"})

    if err := gs.AddMember(g.ID, w.ID, "member"); err != nil {
        t.Fatalf("AddMember 1: %v", err)
    }
    if err := gs.AddMember(g.ID, w.ID, "member"); err != nil {
        t.Fatalf("AddMember 2 (idempotent): %v", err)
    }
}

func TestGroupStore_RemoveMember(t *testing.T) {
    gs, ws := setupGroupTestDB(t)
    g, _ := gs.Create(model.Group{Name: "g", WorkDir: "/tmp/g"})
    w, _ := ws.Create(model.Worker{Name: "w", WorkDir: "/tmp/w"})
    _ = gs.AddMember(g.ID, w.ID, "member")

    if err := gs.RemoveMember(g.ID, w.ID); err != nil {
        t.Fatalf("RemoveMember: %v", err)
    }
    members, _ := gs.ListMembers(g.ID)
    if len(members) != 0 {
        t.Errorf("expected 0 members after remove, got %d", len(members))
    }
}

func TestGroupStore_IsMember(t *testing.T) {
    gs, ws := setupGroupTestDB(t)
    g, _ := gs.Create(model.Group{Name: "g", WorkDir: "/tmp/g"})
    w, _ := ws.Create(model.Worker{Name: "w", WorkDir: "/tmp/w"})

    yes, _ := gs.IsMember(g.ID, w.ID)
    if yes {
        t.Error("expected false before AddMember")
    }
    _ = gs.AddMember(g.ID, w.ID, "member")
    yes, _ = gs.IsMember(g.ID, w.ID)
    if !yes {
        t.Error("expected true after AddMember")
    }
}

func TestGroupStore_ListGroupsForWorker(t *testing.T) {
    gs, ws := setupGroupTestDB(t)
    g1, _ := gs.Create(model.Group{Name: "g1", WorkDir: "/tmp/g1"})
    g2, _ := gs.Create(model.Group{Name: "g2", WorkDir: "/tmp/g2"})
    w, _ := ws.Create(model.Worker{Name: "w", WorkDir: "/tmp/w"})
    _ = gs.AddMember(g1.ID, w.ID, "member")
    _ = gs.AddMember(g2.ID, w.ID, "member")

    groups, err := gs.ListGroupsForWorker(w.ID)
    if err != nil {
        t.Fatalf("ListGroupsForWorker: %v", err)
    }
    if len(groups) != 2 {
        t.Errorf("expected 2 groups, got %d", len(groups))
    }
}

func TestGroupStore_DeleteCascadesMemberships(t *testing.T) {
    gs, ws := setupGroupTestDB(t)
    g, _ := gs.Create(model.Group{Name: "g", WorkDir: "/tmp/g"})
    w, _ := ws.Create(model.Worker{Name: "w", WorkDir: "/tmp/w"})
    _ = gs.AddMember(g.ID, w.ID, "member")

    if err := gs.Delete(g.ID); err != nil {
        t.Fatalf("Delete: %v", err)
    }
    groups, _ := gs.ListGroupsForWorker(w.ID)
    if len(groups) != 0 {
        t.Errorf("expected memberships cleared on group delete, got %d", len(groups))
    }
}
```

- [ ] **Step 4.2: Run — expect compile failure**

```bash
go test ./internal/infra/store/ -run TestGroupStore_ -v
```
Expected: FAIL with undefined methods.

- [ ] **Step 4.3: Implement membership methods**

Append to `internal/infra/store/group_store.go`:

```go
func (s *GroupStore) AddMember(groupID, workerID, role string) error {
    if role == "" {
        role = "member"
    }
    now := time.Now().UnixMilli()
    _, err := s.db.Exec(
        `INSERT OR IGNORE INTO bee_worker_groups (worker_id, group_id, role, created_at)
         VALUES (?, ?, ?, ?)`,
        workerID, groupID, role, now,
    )
    if err != nil {
        return fmt.Errorf("add member: %w", err)
    }
    return nil
}

func (s *GroupStore) RemoveMember(groupID, workerID string) error {
    _, err := s.db.Exec(
        `DELETE FROM bee_worker_groups WHERE group_id = ? AND worker_id = ?`,
        groupID, workerID,
    )
    if err != nil {
        return fmt.Errorf("remove member: %w", err)
    }
    return nil
}

func (s *GroupStore) IsMember(groupID, workerID string) (bool, error) {
    var n int
    err := s.db.QueryRow(
        `SELECT EXISTS(SELECT 1 FROM bee_worker_groups WHERE group_id = ? AND worker_id = ?)`,
        groupID, workerID,
    ).Scan(&n)
    return n == 1, err
}

func (s *GroupStore) ListMembers(groupID string) ([]model.MemberBrief, error) {
    rows, err := s.db.Query(
        `SELECT w.id, w.name, w.description
         FROM bee_workers w
         JOIN bee_worker_groups wg ON wg.worker_id = w.id
         WHERE wg.group_id = ?
         ORDER BY w.created_at ASC`,
        groupID,
    )
    if err != nil {
        return nil, fmt.Errorf("list members: %w", err)
    }
    defer rows.Close()
    var out []model.MemberBrief
    for rows.Next() {
        var m model.MemberBrief
        if err := rows.Scan(&m.ID, &m.Name, &m.Description); err != nil {
            return nil, err
        }
        out = append(out, m)
    }
    return out, rows.Err()
}

func (s *GroupStore) ListGroupsForWorker(workerID string) ([]model.GroupBrief, error) {
    rows, err := s.db.Query(
        `SELECT g.id, g.name
         FROM bee_groups g
         JOIN bee_worker_groups wg ON wg.group_id = g.id
         WHERE wg.worker_id = ?
         ORDER BY g.created_at ASC`,
        workerID,
    )
    if err != nil {
        return nil, fmt.Errorf("list groups for worker: %w", err)
    }
    defer rows.Close()
    var out []model.GroupBrief
    for rows.Next() {
        var b model.GroupBrief
        if err := rows.Scan(&b.ID, &b.Name); err != nil {
            return nil, err
        }
        out = append(out, b)
    }
    return out, rows.Err()
}
```

Now extend the existing `Delete` method to cascade memberships. Replace the current `Delete` body with:

```go
func (s *GroupStore) Delete(id string) error {
    tx, err := s.db.Begin()
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }
    defer tx.Rollback()
    if _, err := tx.Exec(`DELETE FROM bee_worker_groups WHERE group_id = ?`, id); err != nil {
        return fmt.Errorf("delete memberships: %w", err)
    }
    if _, err := tx.Exec(`DELETE FROM bee_groups WHERE id = ?`, id); err != nil {
        return fmt.Errorf("delete group: %w", err)
    }
    return tx.Commit()
}
```

- [ ] **Step 4.4: Run — expect PASS**

```bash
go test ./internal/infra/store/ -run TestGroupStore -v
```
Expected: PASS for all 13 store tests.

- [ ] **Step 4.5: Commit**

```bash
git add internal/infra/store/group_store.go internal/infra/store/group_store_test.go
git commit -m "feat(store): add group membership ops with cascade delete"
```

---

## Task 5: TaskStore additions for task tree

**Files:**
- Modify: `internal/infra/store/task_store.go`
- Modify: `internal/infra/store/task_store_test.go`

- [ ] **Step 5.1: Update `Create` to write the new columns and update `scanTask` to read them**

Find the `Create` SQL in `task_store.go` and replace with:

```go
func (s *TaskStore) Create(ctx context.Context, t model.Task) (string, error) {
    id := uuid.New().String()
    now := time.Now().UnixMilli()
    if t.AgentKind == "" {
        t.AgentKind = model.AgentKindWorker
    }
    if t.RootTaskID == "" {
        t.RootTaskID = id // root tasks self-reference
    }
    _, err := s.db.ExecContext(ctx, `
        INSERT INTO bee_tasks
            (id, message_id, worker_id, instruction, type, status,
             scheduled_at, cron_expr, next_run_at, execution_id,
             parent_task_id, root_task_id, agent_kind,
             created_at, updated_at)
        VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
        id, t.MessageID, t.WorkerID, t.Instruction, t.Type, t.Status,
        t.ScheduledAt, t.CronExpr, t.NextRunAt, "",
        t.ParentTaskID, t.RootTaskID, t.AgentKind,
        now, now,
    )
    if err != nil {
        return "", fmt.Errorf("create task: %w", err)
    }
    return id, nil
}
```

Find the `scanTask` helper and update it to read three new columns. Locate `func scanTask(...)` and update its `Scan(...)` argument list and the SELECT lists in every query in this file (search for `SELECT id, message_id, worker_id, instruction, type, status,`).

The new column order in every SELECT must be:
```
id, message_id, worker_id, instruction, type, status,
scheduled_at, cron_expr, next_run_at, execution_id,
parent_task_id, root_task_id, agent_kind,
created_at, updated_at
```

And the new `scanTask`:

```go
func scanTask(scanner interface{ Scan(...any) error }) (model.Task, error) {
    var t model.Task
    err := scanner.Scan(
        &t.ID, &t.MessageID, &t.WorkerID, &t.Instruction, &t.Type, &t.Status,
        &t.ScheduledAt, &t.CronExpr, &t.NextRunAt, &t.ExecutionID,
        &t.ParentTaskID, &t.RootTaskID, &t.AgentKind,
        &t.CreatedAt, &t.UpdatedAt,
    )
    return t, err
}
```

- [ ] **Step 5.2: Run existing task store tests — expect PASS**

```bash
go test ./internal/infra/store/ -run TestTaskStore -v
```
Expected: PASS. Backwards compatible (defaults handle absent fields).

- [ ] **Step 5.3: Add failing tests for new methods**

Append to `internal/infra/store/task_store_test.go`:

```go
func TestTaskStore_CreateSubtask(t *testing.T) {
    ts, ws, ms := setupTaskTestDB(t) // pattern used in existing tests
    _ = ws
    msg := createMessageHelper(t, ms) // existing helper
    rootID, _ := ts.Create(context.Background(), model.Task{
        MessageID: msg.ID, WorkerID: "g1", Instruction: "root",
        Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
        AgentKind: model.AgentKindGroup,
    })
    subID, err := ts.Create(context.Background(), model.Task{
        MessageID: msg.ID, WorkerID: "w1", Instruction: "sub",
        Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
        ParentTaskID: rootID, RootTaskID: rootID,
        AgentKind: model.AgentKindWorker,
    })
    if err != nil {
        t.Fatalf("Create sub: %v", err)
    }
    sub, _ := ts.GetByID(context.Background(), subID)
    if sub.ParentTaskID != rootID || sub.RootTaskID != rootID {
        t.Errorf("parent/root not persisted: %+v", sub)
    }
    if sub.AgentKind != model.AgentKindWorker {
        t.Errorf("agent_kind not persisted: %s", sub.AgentKind)
    }
}

func TestTaskStore_ListByRoot(t *testing.T) {
    ts, _, ms := setupTaskTestDB(t)
    msg := createMessageHelper(t, ms)
    rootID, _ := ts.Create(context.Background(), model.Task{
        MessageID: msg.ID, WorkerID: "g1", Instruction: "root",
        Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
        AgentKind: model.AgentKindGroup,
    })
    for i := 0; i < 3; i++ {
        _, _ = ts.Create(context.Background(), model.Task{
            MessageID: msg.ID, WorkerID: "w", Instruction: "sub",
            Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
            ParentTaskID: rootID, RootTaskID: rootID,
        })
    }
    tasks, err := ts.ListByRoot(context.Background(), rootID)
    if err != nil {
        t.Fatalf("ListByRoot: %v", err)
    }
    if len(tasks) != 4 { // root + 3 subs
        t.Errorf("expected 4 tasks (root+3), got %d", len(tasks))
    }
}

func TestTaskStore_MarkWaitingSubtasks(t *testing.T) {
    ts, _, ms := setupTaskTestDB(t)
    msg := createMessageHelper(t, ms)
    rootID, _ := ts.Create(context.Background(), model.Task{
        MessageID: msg.ID, WorkerID: "g1", Instruction: "root",
        Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
        AgentKind: model.AgentKindGroup,
    })
    if err := ts.MarkWaitingSubtasks(context.Background(), rootID); err != nil {
        t.Fatalf("MarkWaitingSubtasks: %v", err)
    }
    got, _ := ts.GetByID(context.Background(), rootID)
    if got.Status != model.TaskStatusWaitingSubtasks {
        t.Errorf("expected waiting_subtasks, got %s", got.Status)
    }
}
```

If `setupTaskTestDB` and `createMessageHelper` don't exist by those exact names, look at existing `task_store_test.go` for the equivalent helper and reuse it.

- [ ] **Step 5.4: Run — expect FAIL on `ListByRoot`/`MarkWaitingSubtasks` undefined**

```bash
go test ./internal/infra/store/ -run TestTaskStore_ -v
```
Expected: FAIL with undefined methods.

- [ ] **Step 5.5: Implement the new methods**

Append to `internal/infra/store/task_store.go`:

```go
// ListByRoot returns the entire task tree (root + subtasks) for a given root_task_id.
// Order: root first (created_at ASC).
func (s *TaskStore) ListByRoot(ctx context.Context, rootID string) ([]model.Task, error) {
    rows, err := s.db.QueryContext(ctx, `
        SELECT id, message_id, worker_id, instruction, type, status,
               scheduled_at, cron_expr, next_run_at, execution_id,
               parent_task_id, root_task_id, agent_kind,
               created_at, updated_at
        FROM bee_tasks
        WHERE root_task_id = ?
        ORDER BY created_at ASC`, rootID)
    if err != nil {
        return nil, fmt.Errorf("list by root: %w", err)
    }
    defer rows.Close()
    return scanTasks(rows)
}

// MarkWaitingSubtasks transitions a group root task to waiting_subtasks.
// Returns ErrNoRows if the task does not exist.
func (s *TaskStore) MarkWaitingSubtasks(ctx context.Context, taskID string) error {
    res, err := s.db.ExecContext(ctx,
        `UPDATE bee_tasks SET status = ?, updated_at = ? WHERE id = ?`,
        model.TaskStatusWaitingSubtasks, time.Now().UnixMilli(), taskID,
    )
    if err != nil {
        return fmt.Errorf("mark waiting subtasks: %w", err)
    }
    n, _ := res.RowsAffected()
    if n == 0 {
        return sql.ErrNoRows
    }
    return nil
}

// GetParent returns the parent task, or sql.ErrNoRows if the task is a root.
func (s *TaskStore) GetParent(ctx context.Context, taskID string) (model.Task, error) {
    t, err := s.GetByID(ctx, taskID)
    if err != nil {
        return model.Task{}, err
    }
    if t.ParentTaskID == "" {
        return model.Task{}, sql.ErrNoRows
    }
    return s.GetByID(ctx, t.ParentTaskID)
}
```

Also update the `Status` CHECK constraint mentally — note it already permits any string at the SQL level since the CHECK is enum'd to old values. **This means migration 36 should have been ALTER on the CHECK**. Add an extra migration:

In `db.go`, append migration 39:

```go
{
    version: 39,
    name:    "extend_tasks_status_to_include_waiting_subtasks",
    sql: `-- SQLite cannot ALTER CHECK; recreate table.
CREATE TABLE bee_tasks_new (
    id           TEXT PRIMARY KEY,
    message_id   TEXT NOT NULL REFERENCES bee_platform_messages(id),
    worker_id    TEXT NOT NULL REFERENCES bee_workers(id),
    instruction  TEXT NOT NULL,
    type         TEXT NOT NULL CHECK(type IN ('immediate','countdown','scheduled')),
    status       TEXT NOT NULL DEFAULT 'pending'
        CHECK(status IN ('pending','running','completed','failed','cancelled','waiting_subtasks')),
    scheduled_at INTEGER,
    cron_expr    TEXT NOT NULL DEFAULT '',
    next_run_at  INTEGER,
    execution_id TEXT NOT NULL DEFAULT '',
    parent_task_id TEXT NOT NULL DEFAULT '',
    root_task_id   TEXT NOT NULL DEFAULT '',
    agent_kind     TEXT NOT NULL DEFAULT 'worker'
        CHECK(agent_kind IN ('worker','group')),
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);
INSERT INTO bee_tasks_new SELECT * FROM bee_tasks;
DROP TABLE bee_tasks;
ALTER TABLE bee_tasks_new RENAME TO bee_tasks;
-- Recreate indexes (the rename drops them).
CREATE INDEX idx_tasks_status_type ON bee_tasks(status, type);
CREATE INDEX idx_tasks_message_id ON bee_tasks(message_id);
CREATE INDEX idx_tasks_worker_id ON bee_tasks(worker_id);
CREATE INDEX idx_tasks_parent ON bee_tasks(parent_task_id) WHERE parent_task_id != '';
CREATE INDEX idx_tasks_root ON bee_tasks(root_task_id);`,
},
```

> Note: this migration also relaxes the FK on `worker_id` implicitly. If the FK is desired to remain, it stays — the DDL above keeps the original `REFERENCES bee_workers(id)`. For Group tasks, `worker_id` will hold a Group ID. **Update the migration**: drop the FK on worker_id by removing `REFERENCES bee_workers(id)` from the recreate, since Group IDs live in `bee_groups`.

Replace the `worker_id    TEXT NOT NULL REFERENCES bee_workers(id),` line in migration 39 with:
```sql
worker_id    TEXT NOT NULL,
```

- [ ] **Step 5.6: Run — expect PASS**

```bash
go test ./internal/infra/store/ -v
```
Expected: PASS for all store tests.

- [ ] **Step 5.7: Commit**

```bash
git add internal/infra/store/task_store.go internal/infra/store/task_store_test.go internal/infra/store/db.go
git commit -m "feat(store): add task tree fields and ListByRoot/MarkWaitingSubtasks"
```

---

# Phase 2 — Group domain layer

## Task 6: Group manager — CRUD + name validation

**Files:**
- Create: `internal/domain/group/manager.go`
- Create: `internal/domain/group/manager_test.go`

- [ ] **Step 6.1: Write failing tests**

```go
package group

import (
    "errors"
    "testing"

    "github.com/theopenbee/openbee/internal/infra/model"
)

func TestManager_CreateGroup(t *testing.T) {
    m := newTestManager(t) // helper builds in-memory stores + manager
    g, err := m.CreateGroup(CreateGroupParams{Name: "data-team"})
    if err != nil {
        t.Fatalf("CreateGroup: %v", err)
    }
    if g.ID == "" || g.WorkDir == "" {
        t.Errorf("expected ID and WorkDir, got %+v", g)
    }
}

func TestManager_CreateGroup_NameCollision_WithWorker(t *testing.T) {
    m := newTestManager(t)
    _, _ = m.workerStore.Create(model.Worker{Name: "alpha", WorkDir: "/tmp/w"})
    _, err := m.CreateGroup(CreateGroupParams{Name: "alpha"})
    if err == nil || !errors.Is(err, ErrValidation) {
        t.Errorf("expected ErrValidation on collision with worker, got %v", err)
    }
}

func TestManager_DeleteGroup_RejectsActiveRootTask(t *testing.T) {
    m := newTestManager(t)
    g, _ := m.CreateGroup(CreateGroupParams{Name: "g"})
    // create a fake active root task for this group
    msgID := seedMessage(t, m)
    _, _ = m.taskStore.Create(testCtx(), model.Task{
        MessageID: msgID, WorkerID: g.ID, Instruction: "x",
        Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
        AgentKind: model.AgentKindGroup,
    })
    err := m.DeleteGroup(g.ID, false)
    if err == nil {
        t.Error("expected delete rejection while a root task is running")
    }
}

func TestManager_AddRemoveMember(t *testing.T) {
    m := newTestManager(t)
    g, _ := m.CreateGroup(CreateGroupParams{Name: "g"})
    w, _ := m.workerStore.Create(model.Worker{Name: "w", WorkDir: "/tmp/w"})
    if err := m.AddMember(g.ID, w.ID); err != nil {
        t.Fatalf("AddMember: %v", err)
    }
    members, _ := m.groupStore.ListMembers(g.ID)
    if len(members) != 1 {
        t.Errorf("expected 1 member, got %d", len(members))
    }
    if err := m.RemoveMember(g.ID, w.ID); err != nil {
        t.Fatalf("RemoveMember: %v", err)
    }
}
```

Also create the helper at the top of the file:

```go
func newTestManager(t *testing.T) *Manager {
    t.Helper()
    db, err := store.InitDB(t.TempDir() + "/test.db")
    if err != nil {
        t.Fatalf("InitDB: %v", err)
    }
    t.Cleanup(func() { db.Close() })
    return NewManager(
        t.TempDir(), // groupBaseDir
        store.NewGroupStore(db),
        store.NewWorkerStore(db),
        store.NewTaskStore(db),
        nil, // engines map (not exercised in CRUD tests)
        nil, // engineCfg
        nil, // botNames
    )
}

func seedMessage(t *testing.T, m *Manager) string { /* tiny helper to insert a platform_message row */ }

func testCtx() context.Context { return context.Background() }
```

Add the necessary imports.

- [ ] **Step 6.2: Run — expect FAIL on undefined `Manager`, `NewManager`, `CreateGroupParams`**

```bash
go test ./internal/domain/group/... -v
```
Expected: build failure.

- [ ] **Step 6.3: Implement `manager.go`**

Mirror `internal/domain/worker/manager.go` and `worker.go` patterns:

```go
package group

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "slices"
    "strings"

    "github.com/google/uuid"
    ai "github.com/theopenbee/openbee/internal/ai"
    "github.com/theopenbee/openbee/internal/domain/enginecfg"
    "github.com/theopenbee/openbee/internal/infra/auth"
    "github.com/theopenbee/openbee/internal/infra/model"
    "github.com/theopenbee/openbee/internal/infra/store"
)

var (
    ErrValidation = errors.New("validation")
    ErrNotFound   = errors.New("group not found")
)

type Manager struct {
    groupBaseDir   string
    groupStore     *store.GroupStore
    workerStore    *store.WorkerStore
    taskStore      *store.TaskStore
    engines        map[string]ai.EngineAdapter
    engineCfg      *enginecfg.Store
    botNamesLower  []string
}

func NewManager(
    baseDir string,
    gs *store.GroupStore, ws *store.WorkerStore, ts *store.TaskStore,
    engines map[string]ai.EngineAdapter, engineCfg *enginecfg.Store,
    botNames []string,
) *Manager {
    lower := make([]string, len(botNames))
    for i, n := range botNames {
        lower[i] = strings.ToLower(strings.TrimSpace(n))
    }
    return &Manager{
        groupBaseDir:  baseDir,
        groupStore:    gs,
        workerStore:   ws,
        taskStore:     ts,
        engines:       engines,
        engineCfg:     engineCfg,
        botNamesLower: lower,
    }
}

type CreateGroupParams struct {
    Name             string
    Description      string
    Constraints      string
    WorkDir          string
    PermissionScopes string
    Engine           string
    EngineArgs       string
}

func (m *Manager) CreateGroup(p CreateGroupParams) (model.Group, error) {
    p.Name = strings.TrimSpace(p.Name)
    if err := m.validateName(p.Name, ""); err != nil {
        return model.Group{}, err
    }
    id := uuid.New().String()
    if p.WorkDir == "" {
        p.WorkDir = filepath.Join(m.groupBaseDir, id)
    }
    if err := os.MkdirAll(p.WorkDir, 0o755); err != nil {
        return model.Group{}, fmt.Errorf("create work dir: %w", err)
    }
    engineArgs := p.EngineArgs
    if engineArgs == "" {
        engineArgs = "{}"
    }
    g := model.Group{
        ID:               id,
        Name:             p.Name,
        Description:      p.Description,
        Constraints:      p.Constraints,
        WorkDir:          p.WorkDir,
        Engine:           p.Engine,
        EngineArgs:       engineArgs,
        PermissionScopes: p.PermissionScopes,
    }
    // Optional engine prepare (skipped in tests where engines == nil).
    if m.engines != nil {
        if _, engine, err := m.resolveEngine(g); err == nil {
            if err := engine.Prepare(p.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
                return model.Group{}, fmt.Errorf("prepare group workspace: %w", err)
            }
        }
    }
    return m.groupStore.Create(g)
}

func (m *Manager) DeleteGroup(id string, deleteWorkDir bool) error {
    // Refuse if there are non-terminal root tasks for this group.
    active, err := m.hasActiveRootTask(id)
    if err != nil {
        return err
    }
    if active {
        return fmt.Errorf("group has active root task: %w", ErrValidation)
    }
    if deleteWorkDir {
        g, err := m.groupStore.GetByID(id)
        if err != nil {
            return err
        }
        if g.WorkDir != "" {
            if err := os.RemoveAll(g.WorkDir); err != nil {
                return fmt.Errorf("remove work dir: %w", err)
            }
        }
    }
    return m.groupStore.Delete(id)
}

func (m *Manager) AddMember(groupID, workerID string) error {
    if _, err := m.groupStore.GetByID(groupID); err != nil {
        return ErrNotFound
    }
    if _, err := m.workerStore.GetByID(workerID); err != nil {
        return fmt.Errorf("worker not found: %w", err)
    }
    return m.groupStore.AddMember(groupID, workerID, "member")
}

func (m *Manager) RemoveMember(groupID, workerID string) error {
    return m.groupStore.RemoveMember(groupID, workerID)
}

func (m *Manager) validateName(name, excludeID string) error {
    if name == "" {
        return fmt.Errorf("group name cannot be empty: %w", ErrValidation)
    }
    lower := strings.ToLower(name)
    if slices.Contains(m.botNamesLower, lower) {
        return fmt.Errorf("group name %q conflicts with bot name: %w", name, ErrValidation)
    }
    // Check both group and worker namespaces.
    if exists, err := m.groupStore.ExistsByName(name, excludeID); err != nil {
        return err
    } else if exists {
        return fmt.Errorf("group name %q already taken: %w", name, ErrValidation)
    }
    if exists, err := m.workerStore.ExistsByName(name, ""); err != nil {
        return err
    } else if exists {
        return fmt.Errorf("group name %q conflicts with existing worker: %w", name, ErrValidation)
    }
    return nil
}

func (m *Manager) hasActiveRootTask(groupID string) (bool, error) {
    // Reuse TaskStore.List with worker_id filter and a non-terminal status set.
    tasks, err := m.taskStore.List(context.Background(), store.TaskFilter{
        WorkerID: groupID,
        Status:   "pending,running,waiting_subtasks",
        Limit:    1,
    })
    if err != nil {
        return false, fmt.Errorf("check active tasks: %w", err)
    }
    return len(tasks) > 0, nil
}

func (m *Manager) resolveEngine(g model.Group) (string, ai.EngineAdapter, error) {
    if g.Engine != "" {
        if e, ok := m.engines[g.Engine]; ok {
            return g.Engine, e, nil
        }
    }
    name := m.engineCfg.Get()
    e, ok := m.engines[name]
    if !ok {
        return "", nil, fmt.Errorf("no engine adapter for default %q", name)
    }
    return name, e, nil
}

// Suppress "imported and not used" if auth/sql aren't referenced by something else.
var _ = sql.ErrNoRows
var _ = auth.GenerateBeeToken
```

Drop the trailing `_ = ...` lines if the imports are actually used.

- [ ] **Step 6.4: Run — expect PASS**

```bash
go test ./internal/domain/group/... -v
```
Expected: PASS for the four manager tests.

- [ ] **Step 6.5: Commit**

```bash
git add internal/domain/group/
git commit -m "feat(group): add Manager with CRUD, member ops, and name validation"
```

---

## Task 7: Group persona builder

**Files:**
- Create: `internal/domain/group/persona.go`
- Create: `internal/domain/group/persona_test.go`

- [ ] **Step 7.1: Failing test**

```go
package group

import (
    "strings"
    "testing"

    "github.com/theopenbee/openbee/internal/infra/model"
)

func TestBuildPersona_IncludesNameDescriptionAndConstraints(t *testing.T) {
    g := model.Group{Name: "data-team", Description: "owns ETL", Constraints: "no drops"}
    members := []model.MemberBrief{
        {ID: "w1", Name: "alice", Description: "fetcher"},
        {ID: "w2", Name: "bob", Description: "transformer"},
    }
    out := BuildPersona(g, members)
    for _, want := range []string{"data-team", "owns ETL", "no drops", "alice", "fetcher", "bob", "transformer"} {
        if !strings.Contains(out, want) {
            t.Errorf("persona missing %q:\n%s", want, out)
        }
    }
}

func TestBuildPersona_EmptyMembers(t *testing.T) {
    g := model.Group{Name: "empty", Description: "lonely"}
    out := BuildPersona(g, nil)
    if !strings.Contains(out, "(no members)") {
        t.Errorf("expected '(no members)' marker:\n%s", out)
    }
}
```

- [ ] **Step 7.2: Run — expect FAIL on undefined `BuildPersona`**

```bash
go test ./internal/domain/group/... -run TestBuildPersona -v
```

- [ ] **Step 7.3: Implement**

```go
package group

import (
    "fmt"
    "strings"

    "github.com/theopenbee/openbee/internal/infra/model"
)

// BuildPersona builds the persona block injected into the Group agent's prompt.
// It MUST be deterministic given the same inputs.
func BuildPersona(g model.Group, members []model.MemberBrief) string {
    var sb strings.Builder
    fmt.Fprintf(&sb, "Name: %s\n", g.Name)
    if g.Description != "" {
        fmt.Fprintf(&sb, "Description: %s\n", g.Description)
    }
    if g.Constraints != "" {
        fmt.Fprintf(&sb, "\n## Work Constraints\n%s\n", g.Constraints)
    }
    sb.WriteString("\n## Members\n")
    if len(members) == 0 {
        sb.WriteString("(no members)\n")
    } else {
        for _, m := range members {
            fmt.Fprintf(&sb, "- id=%s name=%s desc=%s\n", m.ID, m.Name, m.Description)
        }
    }
    sb.WriteString("\n## Coordinator Protocol\n")
    sb.WriteString("Use these CLI commands to coordinate sub-tasks:\n")
    sb.WriteString("  openbee ctl task dispatch-subtask --parent-task-id <root> --worker-id <w> --stdin\n")
    sb.WriteString("  openbee ctl task subtasks       --task-id <root>\n")
    sb.WriteString("  openbee ctl task suspend        --task-id <root>\n")
    sb.WriteString("  openbee ctl task mark-success   --task-id <root> [--stdin]\n")
    sb.WriteString("  openbee ctl task mark-failed    --task-id <root> [--stdin]\n")
    sb.WriteString("Each turn: take ONE action set, then call `task suspend` to await sub-task events.\n")
    return sb.String()
}
```

- [ ] **Step 7.4: Run — expect PASS**

```bash
go test ./internal/domain/group/... -run TestBuildPersona -v
```

- [ ] **Step 7.5: Commit**

```bash
git add internal/domain/group/persona.go internal/domain/group/persona_test.go
git commit -m "feat(group): add BuildPersona for runtime prompt injection"
```

---

# Phase 3 — Server-side action layer + CLI surface

## Task 8: Server-side group actions

**Files:**
- Modify: `internal/infra/utils/actions.go` (or wherever `utils.ListWorkers` etc. live)
- Modify: `internal/api/group_handler.go` (new — see Task 9 for handler details)

> Look at existing `utils.ListWorkers` — it's a typed action constant that flows through `ctlRun` to a corresponding API endpoint. Find the file (likely `internal/infra/utils/actions.go`) and follow the same pattern.

- [ ] **Step 8.1: Add new action constants**

In `internal/infra/utils/actions.go`, append:

```go
const (
    CreateGroup       Action = "create_group"
    ListGroups        Action = "list_groups"
    GetGroup          Action = "get_group"
    UpdateGroup       Action = "update_group"
    DeleteGroup       Action = "delete_group"
    AddGroupMember    Action = "add_group_member"
    RemoveGroupMember Action = "remove_group_member"
    ListGroupMembers  Action = "list_group_members"

    DispatchSubtask Action = "dispatch_subtask"
    ListSubtasks    Action = "list_subtasks"
    SuspendTask     Action = "suspend_task"
    MarkTaskSuccess Action = "mark_task_success"
    MarkTaskFailed  Action = "mark_task_failed"
)
```

- [ ] **Step 8.2: Verify it compiles**

```bash
go build ./internal/infra/utils/
```

- [ ] **Step 8.3: Commit**

```bash
git add internal/infra/utils/actions.go
git commit -m "feat(utils): declare group and subtask action constants"
```

---

## Task 9: REST handlers for Group CRUD

**Files:**
- Create: `internal/api/group_handler.go`
- Create: `internal/api/group_handler_test.go`

- [ ] **Step 9.1: Failing test (HTTP-level)**

Mirror `internal/api/department_handler_test.go`. Sample skeleton:

```go
package api

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
)

func TestGroupHandler_CreateAndGet(t *testing.T) {
    h, router := newTestGroupHandler(t)

    body, _ := json.Marshal(map[string]any{"name": "g1"})
    req := httptest.NewRequest(http.MethodPost, "/api/groups", bytes.NewReader(body))
    rec := httptest.NewRecorder()
    router.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK {
        t.Fatalf("create: got %d, body=%s", rec.Code, rec.Body.String())
    }
    var created map[string]any
    _ = json.Unmarshal(rec.Body.Bytes(), &created)
    id := created["id"].(string)

    req = httptest.NewRequest(http.MethodGet, "/api/groups/"+id, nil)
    rec = httptest.NewRecorder()
    router.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK {
        t.Fatalf("get: got %d", rec.Code)
    }
    _ = h
}

func TestGroupHandler_AddRemoveMember(t *testing.T) {
    h, router := newTestGroupHandler(t)
    g, _ := h.manager.CreateGroup(group.CreateGroupParams{Name: "g"})
    w, _ := h.workerStore.Create(model.Worker{Name: "w", WorkDir: "/tmp/w"})

    body, _ := json.Marshal(map[string]any{"worker_id": w.ID})
    req := httptest.NewRequest(http.MethodPost, "/api/groups/"+g.ID+"/members", bytes.NewReader(body))
    rec := httptest.NewRecorder()
    router.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK {
        t.Fatalf("add: got %d", rec.Code)
    }

    req = httptest.NewRequest(http.MethodDelete, "/api/groups/"+g.ID+"/members/"+w.ID, nil)
    rec = httptest.NewRecorder()
    router.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK {
        t.Fatalf("remove: got %d", rec.Code)
    }
}

func newTestGroupHandler(t *testing.T) (*GroupHandler, *gin.Engine) {
    // Initialise an in-memory DB, build a Manager, register the handler routes.
    // See newTestDepartmentHandler in department_handler_test.go for the exact pattern.
    return nil, nil
}
```

- [ ] **Step 9.2: Run — expect FAIL on undefined**

```bash
go test ./internal/api/ -run TestGroupHandler -v
```

- [ ] **Step 9.3: Implement `group_handler.go`**

Mirror `internal/api/department_handler.go` exactly:

```go
package api

import (
    "errors"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/theopenbee/openbee/internal/domain/group"
    "github.com/theopenbee/openbee/internal/infra/model"
    "github.com/theopenbee/openbee/internal/infra/store"
)

type GroupHandler struct {
    manager     *group.Manager
    groupStore  *store.GroupStore
    workerStore *store.WorkerStore
}

func NewGroupHandler(m *group.Manager, gs *store.GroupStore, ws *store.WorkerStore) *GroupHandler {
    return &GroupHandler{manager: m, groupStore: gs, workerStore: ws}
}

func (h *GroupHandler) Create(c *gin.Context) {
    var req struct {
        Name             string `json:"name"`
        Description      string `json:"description"`
        Constraints      string `json:"constraints"`
        Engine           string `json:"engine"`
        EngineArgs       string `json:"engine_args"`
        PermissionScopes string `json:"permission_scopes"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    g, err := h.manager.CreateGroup(group.CreateGroupParams{
        Name:             req.Name,
        Description:      req.Description,
        Constraints:      req.Constraints,
        Engine:           req.Engine,
        EngineArgs:       req.EngineArgs,
        PermissionScopes: req.PermissionScopes,
    })
    if err != nil {
        status := http.StatusInternalServerError
        if errors.Is(err, group.ErrValidation) {
            status = http.StatusBadRequest
        }
        c.JSON(status, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, g)
}

func (h *GroupHandler) List(c *gin.Context) {
    list, err := h.groupStore.List()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    if list == nil {
        list = []model.Group{}
    }
    c.JSON(http.StatusOK, list)
}

func (h *GroupHandler) Get(c *gin.Context) {
    g, err := h.groupStore.GetByID(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
        return
    }
    members, _ := h.groupStore.ListMembers(g.ID)
    c.JSON(http.StatusOK, model.GroupWithMembers{Group: g, Members: members})
}

func (h *GroupHandler) Update(c *gin.Context) {
    g, err := h.groupStore.GetByID(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
        return
    }
    var req struct {
        Name             *string `json:"name"`
        Description      *string `json:"description"`
        Constraints      *string `json:"constraints"`
        Engine           *string `json:"engine"`
        EngineArgs       *string `json:"engine_args"`
        PermissionScopes *string `json:"permission_scopes"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    if req.Name != nil { g.Name = *req.Name }
    if req.Description != nil { g.Description = *req.Description }
    if req.Constraints != nil { g.Constraints = *req.Constraints }
    if req.Engine != nil { g.Engine = *req.Engine }
    if req.EngineArgs != nil { g.EngineArgs = *req.EngineArgs }
    if req.PermissionScopes != nil { g.PermissionScopes = *req.PermissionScopes }
    out, err := h.groupStore.Update(g)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, out)
}

func (h *GroupHandler) Delete(c *gin.Context) {
    deleteWorkDir := c.Query("delete_work_dir") == "true"
    if err := h.manager.DeleteGroup(c.Param("id"), deleteWorkDir); err != nil {
        status := http.StatusInternalServerError
        if errors.Is(err, group.ErrValidation) {
            status = http.StatusBadRequest
        }
        c.JSON(status, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *GroupHandler) AddMember(c *gin.Context) {
    var req struct{ WorkerID string `json:"worker_id"` }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    if err := h.manager.AddMember(c.Param("id"), req.WorkerID); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": "added"})
}

func (h *GroupHandler) RemoveMember(c *gin.Context) {
    if err := h.manager.RemoveMember(c.Param("id"), c.Param("worker_id")); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

func (h *GroupHandler) ListMembers(c *gin.Context) {
    members, err := h.groupStore.ListMembers(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    if members == nil {
        members = []model.MemberBrief{}
    }
    c.JSON(http.StatusOK, members)
}
```

Implement the `newTestGroupHandler` helper in `group_handler_test.go` following `newTestDepartmentHandler` and wire `Create/Get/Update/Delete/AddMember/RemoveMember/ListMembers` to:
```
POST   /api/groups
GET    /api/groups
GET    /api/groups/:id
PUT    /api/groups/:id
DELETE /api/groups/:id
POST   /api/groups/:id/members
DELETE /api/groups/:id/members/:worker_id
GET    /api/groups/:id/members
```

- [ ] **Step 9.4: Run — expect PASS**

```bash
go test ./internal/api/ -run TestGroupHandler -v
```

- [ ] **Step 9.5: Commit**

```bash
git add internal/api/group_handler.go internal/api/group_handler_test.go
git commit -m "feat(api): add Group REST handlers"
```

---

## Task 10: REST handlers for sub-task ops

**Files:**
- Create: `internal/api/subtask_handler.go`
- Create: `internal/api/subtask_handler_test.go`

- [ ] **Step 10.1: Failing test**

```go
package api

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestSubtaskHandler_DispatchSubtask(t *testing.T) {
    h, router := newTestSubtaskHandler(t)
    rootID := h.seedGroupRootTask(t)
    workerID := h.seedWorker(t)

    body, _ := json.Marshal(map[string]any{
        "parent_task_id": rootID,
        "worker_id":      workerID,
        "instruction":    "fetch X",
    })
    req := httptest.NewRequest(http.MethodPost, "/api/tasks/dispatch-subtask", bytes.NewReader(body))
    rec := httptest.NewRecorder()
    router.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK {
        t.Fatalf("got %d, body=%s", rec.Code, rec.Body.String())
    }
    var out map[string]any
    _ = json.Unmarshal(rec.Body.Bytes(), &out)
    if out["subtask_id"] == nil {
        t.Errorf("expected subtask_id in response, got %v", out)
    }
}

func TestSubtaskHandler_Suspend(t *testing.T) {
    h, router := newTestSubtaskHandler(t)
    rootID := h.seedGroupRootTask(t)

    body, _ := json.Marshal(map[string]any{"task_id": rootID})
    req := httptest.NewRequest(http.MethodPost, "/api/tasks/suspend", bytes.NewReader(body))
    rec := httptest.NewRecorder()
    router.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK {
        t.Fatalf("got %d", rec.Code)
    }
    got, _ := h.taskStore.GetByID(testCtx(), rootID)
    if got.Status != "waiting_subtasks" {
        t.Errorf("expected waiting_subtasks, got %s", got.Status)
    }
}
```

(Add similar tests for `subtasks`, `mark-success`, `mark-failed`.)

- [ ] **Step 10.2: Run — expect FAIL**

```bash
go test ./internal/api/ -run TestSubtaskHandler -v
```

- [ ] **Step 10.3: Implement `subtask_handler.go`**

```go
package api

import (
    "io"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/theopenbee/openbee/internal/infra/model"
    "github.com/theopenbee/openbee/internal/infra/store"
)

type SubtaskHandler struct {
    taskStore  *store.TaskStore
    groupStore *store.GroupStore
    notifier   FailureNotifier // existing interface used by other handlers
}

type FailureNotifier interface {
    NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error
}

func NewSubtaskHandler(ts *store.TaskStore, gs *store.GroupStore, n FailureNotifier) *SubtaskHandler {
    return &SubtaskHandler{taskStore: ts, groupStore: gs, notifier: n}
}

func (h *SubtaskHandler) Dispatch(c *gin.Context) {
    var req struct {
        ParentTaskID string `json:"parent_task_id"`
        WorkerID     string `json:"worker_id"`
        Instruction  string `json:"instruction"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    parent, err := h.taskStore.GetByID(c.Request.Context(), req.ParentTaskID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "parent task not found"})
        return
    }
    if parent.AgentKind != model.AgentKindGroup {
        c.JSON(http.StatusBadRequest, gin.H{"error": "parent task is not a group task"})
        return
    }
    subID, err := h.taskStore.Create(c.Request.Context(), model.Task{
        MessageID:    parent.MessageID,
        WorkerID:     req.WorkerID,
        Instruction:  req.Instruction,
        Type:         model.TaskTypeImmediate,
        Status:       model.TaskStatusPending,
        ParentTaskID: parent.ID,
        RootTaskID:   parent.RootTaskID,
        AgentKind:    model.AgentKindWorker,
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"subtask_id": subID})
}

func (h *SubtaskHandler) ListSubtasks(c *gin.Context) {
    rootID := c.Query("task_id")
    if rootID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "task_id required"})
        return
    }
    list, err := h.taskStore.ListByRoot(c.Request.Context(), rootID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, list)
}

func (h *SubtaskHandler) Suspend(c *gin.Context) {
    var req struct{ TaskID string `json:"task_id"` }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    if err := h.taskStore.MarkWaitingSubtasks(c.Request.Context(), req.TaskID); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": "waiting_subtasks"})
}

func (h *SubtaskHandler) MarkSuccess(c *gin.Context) {
    var req struct {
        TaskID string `json:"task_id"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    // Optional result body — currently consumed but not stored beyond the task table's status update.
    _, _ = io.ReadAll(c.Request.Body)
    if err := h.taskStore.CompleteTask(c.Request.Context(), req.TaskID); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": "completed"})
}

func (h *SubtaskHandler) MarkFailed(c *gin.Context) {
    var req struct {
        TaskID string `json:"task_id"`
        Reason string `json:"reason"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    t, err := h.taskStore.GetByID(c.Request.Context(), req.TaskID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
        return
    }
    if err := h.taskStore.FailTask(c.Request.Context(), req.TaskID); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    if h.notifier != nil && t.MessageID != "" {
        _ = h.notifier.NotifyTaskFailure(c.Request.Context(), t.MessageID, model.FailureInfo{
            Reason:     req.Reason,
            WorkerName: t.WorkerID,
        })
    }
    c.JSON(http.StatusOK, gin.H{"status": "failed"})
}
```

> Note: `taskStore.CompleteTask` and `taskStore.FailTask` already exist (used by dispatcher). Verify by `grep -n "func.*TaskStore.*CompleteTask\|func.*TaskStore.*FailTask" internal/infra/store/task_store.go` before implementing.

- [ ] **Step 10.4: Run — expect PASS**

```bash
go test ./internal/api/ -run TestSubtaskHandler -v
```

- [ ] **Step 10.5: Commit**

```bash
git add internal/api/subtask_handler.go internal/api/subtask_handler_test.go
git commit -m "feat(api): add subtask REST handlers (dispatch/suspend/mark-success/mark-failed/list)"
```

---

## Task 11: Wire routes

**Files:**
- Modify: `internal/routes/api.go`

- [ ] **Step 11.1: Register the new routes**

Locate the place where `DepartmentHandler` is registered (search for `departments` in `internal/routes/api.go`) and add immediately after:

```go
// Group routes
groupHandler := api.NewGroupHandler(deps.GroupManager, deps.GroupStore, deps.WorkerStore)
g := r.Group("/api/groups")
{
    g.POST("",                        groupHandler.Create)
    g.GET("",                         groupHandler.List)
    g.GET("/:id",                     groupHandler.Get)
    g.PUT("/:id",                     groupHandler.Update)
    g.DELETE("/:id",                  groupHandler.Delete)
    g.GET("/:id/members",             groupHandler.ListMembers)
    g.POST("/:id/members",            groupHandler.AddMember)
    g.DELETE("/:id/members/:worker_id", groupHandler.RemoveMember)
}

// Subtask routes (used by Group agent CLI calls)
subHandler := api.NewSubtaskHandler(deps.TaskStore, deps.GroupStore, deps.FailureNotifier)
s := r.Group("/api/tasks")
{
    s.POST("/dispatch-subtask", subHandler.Dispatch)
    s.GET("/subtasks",          subHandler.ListSubtasks)
    s.POST("/suspend",          subHandler.Suspend)
    s.POST("/mark-success",     subHandler.MarkSuccess)
    s.POST("/mark-failed",      subHandler.MarkFailed)
}
```

If `deps.GroupManager` and friends don't exist on the deps struct yet, add the fields. Trace through the deps wiring — it propagates from `internal/app/app.go`.

- [ ] **Step 11.2: Verify build**

```bash
go build ./...
```
Expected: success.

- [ ] **Step 11.3: Commit**

```bash
git add internal/routes/api.go
git commit -m "feat(routes): wire group + subtask REST endpoints"
```

---

## Task 12: CLI — `openbee ctl group ...`

**Files:**
- Create: `cmd/openbee/ctl_group.go`

- [ ] **Step 12.1: Implement the cobra command tree**

Mirror `cmd/openbee/ctl_worker.go`:

```go
package main

import (
    "github.com/spf13/cobra"
    "github.com/theopenbee/openbee/internal/infra/utils"
)

var ctlGroupCmd = &cobra.Command{Use: "group", Short: ""}

var (
    groupCreateName        string
    groupCreateDescription string
    groupCreateConstraints string
    groupCreateEngine      string
    groupCreateScopes      string
)

var ctlGroupCreateCmd = &cobra.Command{
    Use:   "create",
    Short: "Create a new group",
    RunE: func(cmd *cobra.Command, args []string) error {
        a := map[string]any{"name": groupCreateName}
        if groupCreateDescription != "" { a["description"] = groupCreateDescription }
        if groupCreateConstraints != "" { a["constraints"] = groupCreateConstraints }
        if groupCreateEngine != ""      { a["engine"] = groupCreateEngine }
        if groupCreateScopes != ""      { a["permission_scopes"] = groupCreateScopes }
        return ctlRun(utils.CreateGroup, a)
    },
}

var ctlGroupListCmd = &cobra.Command{
    Use:   "list",
    Short: "List groups",
    RunE: func(cmd *cobra.Command, args []string) error {
        return ctlRun(utils.ListGroups, map[string]any{})
    },
}

var ctlGroupGetCmd = &cobra.Command{
    Use:   "get <id>",
    Short: "Get a group by ID",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        return ctlRun(utils.GetGroup, map[string]any{"group_id": args[0]})
    },
}

var ctlGroupDeleteCmd = &cobra.Command{
    Use:   "delete <id>",
    Short: "Delete a group",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        a := map[string]any{"group_id": args[0]}
        if groupDeleteWorkDir { a["delete_work_dir"] = true }
        return ctlRun(utils.DeleteGroup, a)
    },
}

var groupDeleteWorkDir bool

var ctlGroupMemberCmd = &cobra.Command{Use: "member", Short: ""}

var (
    groupMemberGroupID  string
    groupMemberWorkerID string
)

var ctlGroupMemberAddCmd = &cobra.Command{
    Use:   "add",
    Short: "Add a worker to a group",
    RunE: func(cmd *cobra.Command, args []string) error {
        return ctlRun(utils.AddGroupMember, map[string]any{
            "group_id":  groupMemberGroupID,
            "worker_id": groupMemberWorkerID,
        })
    },
}

var ctlGroupMemberRemoveCmd = &cobra.Command{
    Use:   "remove",
    Short: "Remove a worker from a group",
    RunE: func(cmd *cobra.Command, args []string) error {
        return ctlRun(utils.RemoveGroupMember, map[string]any{
            "group_id":  groupMemberGroupID,
            "worker_id": groupMemberWorkerID,
        })
    },
}

var ctlGroupMemberListCmd = &cobra.Command{
    Use:   "list",
    Short: "List members of a group",
    RunE: func(cmd *cobra.Command, args []string) error {
        return ctlRun(utils.ListGroupMembers, map[string]any{"group_id": groupMemberGroupID})
    },
}

func init() {
    ctlGroupCreateCmd.Flags().StringVar(&groupCreateName, "name", "", "Group name (required)")
    ctlGroupCreateCmd.Flags().StringVar(&groupCreateDescription, "description", "", "Group description")
    ctlGroupCreateCmd.Flags().StringVar(&groupCreateConstraints, "constraints", "", "Work constraints")
    ctlGroupCreateCmd.Flags().StringVar(&groupCreateEngine, "engine", "", "Engine name override")
    ctlGroupCreateCmd.Flags().StringVar(&groupCreateScopes, "permission-scopes", "", "Permission scopes")
    ctlGroupCreateCmd.MarkFlagRequired("name")

    ctlGroupDeleteCmd.Flags().BoolVar(&groupDeleteWorkDir, "delete-work-dir", false, "Also delete the group's work_dir on disk")

    for _, c := range []*cobra.Command{ctlGroupMemberAddCmd, ctlGroupMemberRemoveCmd, ctlGroupMemberListCmd} {
        c.Flags().StringVar(&groupMemberGroupID, "group", "", "Group ID (required)")
        c.MarkFlagRequired("group")
    }
    ctlGroupMemberAddCmd.Flags().StringVar(&groupMemberWorkerID, "worker", "", "Worker ID (required)")
    ctlGroupMemberAddCmd.MarkFlagRequired("worker")
    ctlGroupMemberRemoveCmd.Flags().StringVar(&groupMemberWorkerID, "worker", "", "Worker ID (required)")
    ctlGroupMemberRemoveCmd.MarkFlagRequired("worker")

    ctlGroupMemberCmd.AddCommand(ctlGroupMemberAddCmd, ctlGroupMemberRemoveCmd, ctlGroupMemberListCmd)
    ctlGroupCmd.AddCommand(ctlGroupCreateCmd, ctlGroupListCmd, ctlGroupGetCmd, ctlGroupDeleteCmd, ctlGroupMemberCmd)
    ctlCmd.AddCommand(ctlGroupCmd)
}
```

- [ ] **Step 12.2: Verify build**

```bash
go build ./cmd/openbee/
```

- [ ] **Step 12.3: Commit**

```bash
git add cmd/openbee/ctl_group.go
git commit -m "feat(cli): add openbee ctl group command tree"
```

---

## Task 13: CLI — `openbee ctl task` sub-task verbs

**Files:**
- Modify: `cmd/openbee/ctl_task.go`

- [ ] **Step 13.1: Add 5 sub-commands**

Append to `cmd/openbee/ctl_task.go`:

```go
import "io"
import "os"

var (
    taskDispatchSubParent string
    taskDispatchSubWorker string
    taskDispatchSubFromStdin bool
)

var ctlTaskDispatchSubtaskCmd = &cobra.Command{
    Use:   "dispatch-subtask",
    Short: "Group coordinator: create and dispatch a sub-task to a member worker",
    RunE: func(cmd *cobra.Command, args []string) error {
        instruction := ""
        if taskDispatchSubFromStdin {
            b, err := io.ReadAll(os.Stdin)
            if err != nil {
                return err
            }
            instruction = string(b)
        }
        return ctlRun(utils.DispatchSubtask, map[string]any{
            "parent_task_id": taskDispatchSubParent,
            "worker_id":      taskDispatchSubWorker,
            "instruction":    instruction,
        })
    },
}

var taskSubtasksTaskID string

var ctlTaskSubtasksCmd = &cobra.Command{
    Use:   "subtasks",
    Short: "List all subtasks under a root task",
    RunE: func(cmd *cobra.Command, args []string) error {
        return ctlRun(utils.ListSubtasks, map[string]any{"task_id": taskSubtasksTaskID})
    },
}

var taskSuspendTaskID string

var ctlTaskSuspendCmd = &cobra.Command{
    Use:   "suspend",
    Short: "Group coordinator: mark the root task waiting_subtasks and exit",
    RunE: func(cmd *cobra.Command, args []string) error {
        return ctlRun(utils.SuspendTask, map[string]any{"task_id": taskSuspendTaskID})
    },
}

var (
    taskMarkSuccessTaskID string
    taskMarkSuccessStdin  bool
    taskMarkFailedTaskID  string
    taskMarkFailedStdin   bool
)

func readOptionalStdin(flag bool) string {
    if !flag { return "" }
    b, _ := io.ReadAll(os.Stdin)
    return string(b)
}

var ctlTaskMarkSuccessCmd = &cobra.Command{
    Use:   "mark-success",
    Short: "Group coordinator: declare the root task complete",
    RunE: func(cmd *cobra.Command, args []string) error {
        return ctlRun(utils.MarkTaskSuccess, map[string]any{
            "task_id": taskMarkSuccessTaskID,
            "result":  readOptionalStdin(taskMarkSuccessStdin),
        })
    },
}

var ctlTaskMarkFailedCmd = &cobra.Command{
    Use:   "mark-failed",
    Short: "Group coordinator: declare the root task failed",
    RunE: func(cmd *cobra.Command, args []string) error {
        return ctlRun(utils.MarkTaskFailed, map[string]any{
            "task_id": taskMarkFailedTaskID,
            "reason":  readOptionalStdin(taskMarkFailedStdin),
        })
    },
}

func init() {
    ctlTaskDispatchSubtaskCmd.Flags().StringVar(&taskDispatchSubParent, "parent-task-id", "", "Root task ID (required)")
    ctlTaskDispatchSubtaskCmd.Flags().StringVar(&taskDispatchSubWorker, "worker-id", "", "Member worker ID (required)")
    ctlTaskDispatchSubtaskCmd.Flags().BoolVar(&taskDispatchSubFromStdin, "stdin", false, "Read instruction from stdin")
    ctlTaskDispatchSubtaskCmd.MarkFlagRequired("parent-task-id")
    ctlTaskDispatchSubtaskCmd.MarkFlagRequired("worker-id")

    ctlTaskSubtasksCmd.Flags().StringVar(&taskSubtasksTaskID, "task-id", "", "Root task ID (required)")
    ctlTaskSubtasksCmd.MarkFlagRequired("task-id")

    ctlTaskSuspendCmd.Flags().StringVar(&taskSuspendTaskID, "task-id", "", "Root task ID (required)")
    ctlTaskSuspendCmd.MarkFlagRequired("task-id")

    ctlTaskMarkSuccessCmd.Flags().StringVar(&taskMarkSuccessTaskID, "task-id", "", "Root task ID (required)")
    ctlTaskMarkSuccessCmd.Flags().BoolVar(&taskMarkSuccessStdin, "stdin", false, "Read result from stdin")
    ctlTaskMarkSuccessCmd.MarkFlagRequired("task-id")

    ctlTaskMarkFailedCmd.Flags().StringVar(&taskMarkFailedTaskID, "task-id", "", "Root task ID (required)")
    ctlTaskMarkFailedCmd.Flags().BoolVar(&taskMarkFailedStdin, "stdin", false, "Read failure reason from stdin")
    ctlTaskMarkFailedCmd.MarkFlagRequired("task-id")

    ctlTaskCmd.AddCommand(
        ctlTaskDispatchSubtaskCmd,
        ctlTaskSubtasksCmd,
        ctlTaskSuspendCmd,
        ctlTaskMarkSuccessCmd,
        ctlTaskMarkFailedCmd,
    )
}
```

- [ ] **Step 13.2: Verify build**

```bash
go build ./cmd/openbee/
```

- [ ] **Step 13.3: Commit**

```bash
git add cmd/openbee/ctl_task.go
git commit -m "feat(cli): add task sub-commands for group coordination"
```

---

# Phase 4 — Dispatcher integration

## Task 14: `agent_kind` branching in dispatcher

**Files:**
- Modify: `internal/domain/task/dispatcher.go`
- Modify: `internal/domain/task/dispatcher_internal_test.go`

- [ ] **Step 14.1: Add a `GroupLookup` interface**

In `dispatcher.go`, add near the existing `WorkerLookup`:

```go
// GroupLookup fetches group metadata for persona injection.
type GroupLookup interface {
    GetByID(id string) (model.Group, error)
    ListMembers(groupID string) ([]model.MemberBrief, error)
}

// WithGroupLookup wires Group persona injection.
func WithGroupLookup(lookup GroupLookup) Option {
    return func(d *TaskDispatcher) { d.groupLookup = lookup }
}
```

Add the field on `TaskDispatcher`:
```go
groupLookup GroupLookup
```

- [ ] **Step 14.2: Branch on `agent_kind` in `executeWithHint`**

Replace `executeWithHint` body to look up group + members + persona when applicable:

```go
func (d *TaskDispatcher) executeWithHint(ctx context.Context, task DispatchTask, instruction, engineName string, worker *model.Worker) (model.WorkerExecution, error) {
    // Detect group task by checking the persisted task row's agent_kind.
    isGroup := false
    var groupID string
    if t, err := d.taskStoreFull().GetByID(ctx, task.TaskID); err == nil {
        if t.AgentKind == model.AgentKindGroup {
            isGroup = true
            groupID = t.WorkerID
        }
    }

    hint := ai.SkillHintPrefix(ai.RoleWorker)
    if isGroup && d.groupLookup != nil {
        g, err := d.groupLookup.GetByID(groupID)
        if err != nil {
            return model.WorkerExecution{}, fmt.Errorf("group lookup: %w", err)
        }
        members, _ := d.groupLookup.ListMembers(groupID)
        hint += "\n<group_persona>\n" + group.BuildPersona(g, members) + "</group_persona>"
    } else if d.workerLookup != nil {
        if worker == nil {
            return model.WorkerExecution{}, fmt.Errorf("worker %q not found", task.WorkerID)
        }
        persona := ai.WorkerPersona(worker.Name, worker.Description, worker.Constraints)
        hint += "\n<worker_persona>\n" + persona + "</worker_persona>"
    }
    sessionID := uuid.New().String()
    d.upsertSessionContext(ctx, task, sessionID, engineName)
    log.Info("executing agent", zap.String("agentID", task.WorkerID), zap.String("taskID", task.TaskID), zap.Bool("group", isGroup))
    return d.manager.ExecuteWorker(ctx, task.WorkerID, hint+"\n"+instruction, sessionID, false)
}
```

> Add an interface for the full task store on TaskDispatcher (`taskStoreFull`) — currently dispatcher has only the narrow `TaskStore` interface that lacks `GetByID`. Extend that interface or add a separate one:

```go
type TaskQuerier interface {
    GetByID(ctx context.Context, id string) (model.Task, error)
}
```

Wire a `taskQuerier TaskQuerier` field and add a corresponding `WithTaskQuerier` option.

- [ ] **Step 14.3: Test — group task path uses Group persona**

Add a `dispatcher_internal_test.go` test that:
- builds a fake taskQuerier returning `agent_kind=group`
- builds a fake groupLookup returning a known Group + members
- runs `handleInbound` with a group task
- asserts the prompt sent to fake `ExecuteWorker` contains `<group_persona>` and the member names.

Run:
```bash
go test ./internal/domain/task/ -v -run TestGroupPersonaInjection
```
Expected: PASS.

- [ ] **Step 14.4: Commit**

```bash
git add internal/domain/task/dispatcher.go internal/domain/task/dispatcher_internal_test.go
git commit -m "feat(dispatcher): branch on agent_kind, inject group persona"
```

---

## Task 15: Sub-task event → parent resume

**Files:**
- Modify: `internal/domain/task/dispatcher.go`
- Modify: `internal/domain/task/dispatcher_internal_test.go`

- [ ] **Step 15.1: Add `notifyParentOnSubtaskTerminal` helper**

Append to `dispatcher.go`:

```go
// notifyParentOnSubtaskTerminal: when a sub-task reaches a terminal state, build
// a snapshot of the entire root task tree and re-enqueue a DispatchTask
// targeting the parent (Group) session so it can be resumed.
func (d *TaskDispatcher) notifyParentOnSubtaskTerminal(ctx context.Context, finishedTask DispatchTask) {
    if d.taskQuerier == nil { return }
    sub, err := d.taskQuerier.GetByID(ctx, finishedTask.TaskID)
    if err != nil || sub.ParentTaskID == "" {
        return
    }
    parent, err := d.taskQuerier.GetByID(ctx, sub.ParentTaskID)
    if err != nil {
        log.Error("get parent task", zap.Error(err))
        return
    }
    if parent.Status != model.TaskStatusWaitingSubtasks &&
        parent.Status != model.TaskStatusRunning {
        return
    }
    // Build the snapshot.
    snapshot := d.buildSubtaskEventXML(ctx, parent.RootTaskID, sub)
    // Synthesise a DispatchTask that re-enters the dispatcher's queue, keyed by
    // the Group's worker_id so the existing per-agent serialization wins.
    select {
    case d.subtaskEventCh <- DispatchTask{
        TaskID:     parent.ID,
        WorkerID:   parent.WorkerID,
        SessionKey: finishedTask.SessionKey, // user's original session_key
        Instruction: snapshot,
        TaskType:   model.TaskTypeImmediate, // resume path
        MessageID:  parent.MessageID,
    }:
    default:
        log.Warn("subtaskEventCh full, dropping resume signal", zap.String("parentTaskID", parent.ID))
    }
}

func (d *TaskDispatcher) buildSubtaskEventXML(ctx context.Context, rootID string, recent model.Task) string {
    list, _ := d.taskQuerier.ListByRoot(ctx, rootID) // extend interface
    var sb strings.Builder
    sb.WriteString("<subtask_event>\n")
    fmt.Fprintf(&sb, "<root_task id=\"%s\"/>\n", rootID)
    sb.WriteString("<subtasks>\n")
    for _, t := range list {
        if t.ID == rootID { continue }
        fmt.Fprintf(&sb, "  <subtask id=\"%s\" worker=\"%s\" status=\"%s\"/>\n", t.ID, t.WorkerID, t.Status)
    }
    sb.WriteString("</subtasks>\n")
    fmt.Fprintf(&sb, "<recent id=\"%s\" status=\"%s\"/>\n", recent.ID, recent.Status)
    sb.WriteString("</subtask_event>\n")
    return sb.String()
}
```

Extend the `TaskQuerier` interface to include `ListByRoot`:

```go
type TaskQuerier interface {
    GetByID(ctx context.Context, id string) (model.Task, error)
    ListByRoot(ctx context.Context, rootID string) ([]model.Task, error)
}
```

Add the `subtaskEventCh chan DispatchTask` field, init it in `New`:
```go
subtaskEventCh: make(chan DispatchTask, 256),
```

In the `Run` select loop, add a case to drain it:
```go
case ev := <-d.subtaskEventCh:
    d.handleInbound(ev)
```

- [ ] **Step 15.2: Hook the helper into `waitForResult`**

In the `case model.ExecStatusCompleted` and `case model.ExecStatusFailed` branches, after the existing `CompleteTask`/`FailTask` calls, **but only when the task has a parent**, swap the user-facing notification for the parent-resume helper. Replace the relevant blocks with:

```go
case model.ExecStatusCompleted:
    if task.TaskID != "" {
        if err := d.taskStore.CompleteTask(ctx, task.TaskID); err != nil {
            log.Error("complete task", zap.String("taskID", task.TaskID), zap.Error(err))
        }
    }
    d.upsertSessionContext(ctx, task, exec.SessionID, engineName)
    if d.taskHasParent(task.TaskID) {
        d.notifyParentOnSubtaskTerminal(ctx, task) // resume parent, do not notify user
    }
    return
case model.ExecStatusFailed:
    d.upsertSessionContext(ctx, task, exec.SessionID, engineName)
    if task.TaskID != "" {
        if err := d.taskStore.FailTask(ctx, task.TaskID); err != nil {
            log.Error("fail task", zap.String("taskID", task.TaskID), zap.Error(err))
        }
    }
    if d.taskHasParent(task.TaskID) {
        d.notifyParentOnSubtaskTerminal(ctx, task)
        return
    }
    d.notifyFailure(ctx, task.MessageID, model.FailureInfo{
        Reason:     exec.Result,
        WorkerName: workerName(exec.WorkerName, task.WorkerID),
    })
    return
```

Implement `taskHasParent`:
```go
func (d *TaskDispatcher) taskHasParent(taskID string) bool {
    if d.taskQuerier == nil { return false }
    t, err := d.taskQuerier.GetByID(context.Background(), taskID)
    if err != nil { return false }
    return t.ParentTaskID != ""
}
```

- [ ] **Step 15.3: Failing test — sub-task completion triggers parent resume**

Add `TestSubtaskCompletionResumesParent` to `dispatcher_internal_test.go`:

```go
func TestSubtaskCompletionResumesParent(t *testing.T) {
    rootID := "root1"
    subID  := "sub1"
    fakeQuerier := newFakeTaskQuerier(map[string]model.Task{
        rootID: {ID: rootID, WorkerID: "g1", AgentKind: model.AgentKindGroup, Status: model.TaskStatusWaitingSubtasks, MessageID: "m1"},
        subID:  {ID: subID,  WorkerID: "w1", AgentKind: model.AgentKindWorker, Status: model.TaskStatusCompleted, ParentTaskID: rootID, RootTaskID: rootID},
    })
    fakeMgr := newFakeExecMgr()
    d := New(fakeMgr, fakeStore{}, fakeSessStore{}, fakeExecStore{}, make(chan DispatchTask), nil, WithTaskQuerier(fakeQuerier))
    go d.Run(context.Background())

    // Simulate sub-task finished:
    d.subtaskEventCh = make(chan DispatchTask, 4)
    d.notifyParentOnSubtaskTerminal(context.Background(), DispatchTask{TaskID: subID, SessionKey: "session-x"})

    select {
    case ev := <-d.subtaskEventCh:
        if ev.TaskID != rootID {
            t.Errorf("expected resume targeting root %s, got %s", rootID, ev.TaskID)
        }
        if !strings.Contains(ev.Instruction, "<subtask_event>") {
            t.Errorf("expected subtask_event in instruction, got %q", ev.Instruction)
        }
    case <-time.After(time.Second):
        t.Fatal("no resume event observed")
    }
}
```

(Adapt to the existing fakes signatures — see `dispatcher_internal_test.go` for the exact constructors.)

- [ ] **Step 15.4: Run — expect PASS**

```bash
go test ./internal/domain/task/ -run TestSubtaskCompletionResumesParent -v
```

- [ ] **Step 15.5: Commit**

```bash
git add internal/domain/task/dispatcher.go internal/domain/task/dispatcher_internal_test.go
git commit -m "feat(dispatcher): resume Group session on sub-task terminal events"
```

---

## Task 16: Worker `message send` server-side rerouting

**Files:**
- Modify: `internal/api/message_handler.go`
- Modify: `internal/api/message_handler_test.go`

- [ ] **Step 16.1: Failing test**

Append to `message_handler_test.go`:

```go
func TestMessageSend_SubtaskReroutesToParent(t *testing.T) {
    h, router := newTestMessageHandler(t)
    rootID := h.seedGroupRootTask(t)
    subID  := h.seedSubtask(t, rootID)
    msgID  := h.seedMessageForTask(t, subID)

    body, _ := json.Marshal(map[string]any{
        "message_id": msgID,
        "content":    "subtask says hi",
    })
    req := httptest.NewRequest(http.MethodPost, "/api/messages/send", bytes.NewReader(body))
    rec := httptest.NewRecorder()
    router.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK {
        t.Fatalf("got %d, body=%s", rec.Code, rec.Body.String())
    }
    // Outbound store should NOT have a new platform message.
    outbound, _ := h.outboundStore.ListByMessageID(msgID)
    if len(outbound) != 0 {
        t.Errorf("expected no IM outbound for sub-task, got %d", len(outbound))
    }
    // The parent-resume channel should have received an event.
    if !h.dispatcher.LastResumeWasFor(rootID) {
        t.Error("expected parent resume to be triggered")
    }
}
```

- [ ] **Step 16.2: Implement the reroute**

In `message_handler.go`'s `Send` function (or whatever `message send` resolves to server-side), after binding the request and **before** writing to the outbound store, look up the originating task and branch:

```go
// Detect sub-task context: if any task with this message_id has parent_task_id,
// reroute to the parent session instead of sending to IM.
tasks, _ := h.taskStore.List(ctx, store.TaskFilter{MessageID: req.MessageID, Limit: 1})
if len(tasks) == 1 && tasks[0].ParentTaskID != "" {
    h.dispatcher.NotifySubtaskProgress(ctx, tasks[0], req.Content)
    c.JSON(http.StatusOK, gin.H{"status": "rerouted_to_parent"})
    return
}
// Existing IM-platform send flow continues here.
```

Add `NotifySubtaskProgress` on `TaskDispatcher` as a public method that pushes a `<subtask_event source=worker_message>...content...</subtask_event>` into `subtaskEventCh`.

- [ ] **Step 16.3: Run — expect PASS**

```bash
go test ./internal/api/ -run TestMessageSend_Subtask -v
```

- [ ] **Step 16.4: Commit**

```bash
git add internal/api/message_handler.go internal/api/message_handler_test.go internal/domain/task/dispatcher.go
git commit -m "feat(api): reroute Worker message send to Group when in sub-task context"
```

---

## Task 17: Phantom suspend detection

**Files:**
- Modify: `internal/api/subtask_handler.go`
- Modify: `internal/api/subtask_handler_test.go`

- [ ] **Step 17.1: Failing test**

```go
func TestSubtaskHandler_Suspend_AllSubtasksDone_TriggersImmediateResume(t *testing.T) {
    h, router := newTestSubtaskHandler(t)
    rootID := h.seedGroupRootTask(t)
    h.seedTerminalSubtask(t, rootID, "completed")
    h.seedTerminalSubtask(t, rootID, "completed")

    body, _ := json.Marshal(map[string]any{"task_id": rootID})
    req := httptest.NewRequest(http.MethodPost, "/api/tasks/suspend", bytes.NewReader(body))
    rec := httptest.NewRecorder()
    router.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK {
        t.Fatalf("got %d", rec.Code)
    }
    if !h.dispatcher.LastResumeWasFor(rootID) {
        t.Error("expected immediate resume because all subtasks already terminal")
    }
}
```

- [ ] **Step 17.2: Implement detection**

In `Suspend` handler, after `MarkWaitingSubtasks` succeeds:

```go
list, _ := h.taskStore.ListByRoot(c.Request.Context(), req.TaskID)
allTerminal := true
for _, t := range list {
    if t.ID == req.TaskID { continue }
    if t.Status == model.TaskStatusPending ||
        t.Status == model.TaskStatusRunning ||
        t.Status == model.TaskStatusWaitingSubtasks {
        allTerminal = false; break
    }
}
if allTerminal && len(list) > 1 {
    h.dispatcher.NotifyAllSubtasksTerminal(c.Request.Context(), req.TaskID)
}
```

Add `NotifyAllSubtasksTerminal` on TaskDispatcher that synthesises a `<subtask_event status="all_done">` and pushes to `subtaskEventCh`.

- [ ] **Step 17.3: Run — expect PASS**

```bash
go test ./internal/api/ -run TestSubtaskHandler_Suspend_AllSubtasksDone -v
```

- [ ] **Step 17.4: Commit**

```bash
git add internal/api/subtask_handler.go internal/api/subtask_handler_test.go internal/domain/task/dispatcher.go
git commit -m "feat(dispatcher): trigger immediate resume on phantom suspend"
```

---

## Task 18: Cancellation cascade

**Files:**
- Modify: `internal/domain/task/dispatcher.go`
- Modify: `internal/domain/task/dispatcher_internal_test.go`

- [ ] **Step 18.1: Failing test**

```go
func TestCancelRootTask_CascadesToSubtasks(t *testing.T) {
    rootID := "r1"
    sub1, sub2 := "s1", "s2"
    fq := newFakeTaskQuerier(map[string]model.Task{
        rootID: {ID: rootID, WorkerID: "g", AgentKind: model.AgentKindGroup, Status: model.TaskStatusWaitingSubtasks},
        sub1:   {ID: sub1, ParentTaskID: rootID, RootTaskID: rootID, Status: model.TaskStatusRunning},
        sub2:   {ID: sub2, ParentTaskID: rootID, RootTaskID: rootID, Status: model.TaskStatusPending},
    })
    fakeStore := &spyTaskStore{}
    d := New(newFakeExecMgr(), fakeStore, fakeSessStore{}, fakeExecStore{}, make(chan DispatchTask), nil, WithTaskQuerier(fq))
    go d.Run(context.Background())

    if err := d.CancelTask(context.Background(), rootID); err != nil {
        t.Fatalf("CancelTask: %v", err)
    }
    time.Sleep(50 * time.Millisecond)
    if !fakeStore.cancelled[sub1] || !fakeStore.cancelled[sub2] {
        t.Errorf("expected sub1 and sub2 cancelled, got %+v", fakeStore.cancelled)
    }
}
```

- [ ] **Step 18.2: Implement cascade in `handleCancel`**

Extend the existing `handleCancel`:

```go
func (d *TaskDispatcher) handleCancel(taskID string) {
    // If this is a root task with sub-tasks, cascade.
    if d.taskQuerier != nil {
        if t, err := d.taskQuerier.GetByID(context.Background(), taskID); err == nil && t.AgentKind == model.AgentKindGroup {
            children, _ := d.taskQuerier.ListByRoot(context.Background(), taskID)
            for _, ch := range children {
                if ch.ID == taskID { continue }
                if ch.Status == model.TaskStatusPending || ch.Status == model.TaskStatusRunning {
                    _ = d.taskStore.CancelTask(context.Background(), ch.ID)
                    if cancel, ok := d.cancelFuncs[ch.ID]; ok {
                        cancel(); delete(d.cancelFuncs, ch.ID)
                    }
                }
            }
        }
    }
    // Existing per-task cancel behaviour:
    for key, state := range d.queues {
        var remaining []DispatchTask
        for _, t := range state.pendingTasks {
            if t.TaskID != taskID { remaining = append(remaining, t) }
        }
        state.pendingTasks = remaining
        if !state.executing && len(state.pendingTasks) == 0 {
            delete(d.queues, key)
        }
    }
    if cancel, ok := d.cancelFuncs[taskID]; ok {
        cancel(); delete(d.cancelFuncs, taskID)
    }
}
```

- [ ] **Step 18.3: Run — expect PASS**

```bash
go test ./internal/domain/task/ -run TestCancelRootTask -v
```

- [ ] **Step 18.4: Commit**

```bash
git add internal/domain/task/dispatcher.go internal/domain/task/dispatcher_internal_test.go
git commit -m "feat(dispatcher): cascade cancel from root task to subtasks"
```

---

# Phase 5 — Crash recovery

## Task 19: `RecoverGroupTasks`

**Files:**
- Create: `internal/domain/task/recovery.go`
- Create: `internal/domain/task/recovery_test.go`

- [ ] **Step 19.1: Failing test**

```go
package task

import (
    "context"
    "testing"

    "github.com/theopenbee/openbee/internal/infra/model"
)

func TestRecoverGroupTasks_ResumeWaitingRoots(t *testing.T) {
    rootID := "r1"
    fq := newFakeTaskQuerier(map[string]model.Task{
        rootID: {ID: rootID, WorkerID: "g1", AgentKind: model.AgentKindGroup, Status: model.TaskStatusWaitingSubtasks, MessageID: "m1"},
    })
    fs := newFakeSessStore(map[string]string{
        "session-x|g1|claude": "session-id-existing",
    })
    out := make(chan DispatchTask, 4)
    err := RecoverGroupTasks(context.Background(), fq, fs, out, "claude")
    if err != nil { t.Fatalf("RecoverGroupTasks: %v", err) }
    select {
    case ev := <-out:
        if ev.TaskID != rootID || !strings.Contains(ev.Instruction, "<recovery_event>") {
            t.Errorf("unexpected event: %+v", ev)
        }
    default:
        t.Fatal("no recovery event emitted")
    }
}
```

- [ ] **Step 19.2: Implement**

```go
package task

import (
    "context"
    "fmt"

    "github.com/theopenbee/openbee/internal/infra/model"
)

type recoveryStore interface {
    ListWaitingGroupRoots(ctx context.Context) ([]model.Task, error)
    ListByRoot(ctx context.Context, rootID string) ([]model.Task, error)
}

type recoverySessionStore interface {
    SessionKeyForAgent(ctx context.Context, agentID, engine string) (string, string, bool, error)
}

func RecoverGroupTasks(ctx context.Context, ts recoveryStore, ss recoverySessionStore, out chan<- DispatchTask, engineName string) error {
    roots, err := ts.ListWaitingGroupRoots(ctx)
    if err != nil { return fmt.Errorf("list waiting roots: %w", err) }
    for _, root := range roots {
        sessionKey, _, ok, err := ss.SessionKeyForAgent(ctx, root.WorkerID, engineName)
        if err != nil || !ok {
            continue // session lost — separate path could fail-the-task; deferred
        }
        snapshot := buildRecoveryEventXML(ctx, ts, root)
        select {
        case out <- DispatchTask{
            TaskID:      root.ID,
            WorkerID:    root.WorkerID,
            SessionKey:  sessionKey,
            Instruction: snapshot,
            TaskType:    model.TaskTypeImmediate,
            MessageID:   root.MessageID,
        }:
        default:
            log.Warn("recovery channel full")
        }
    }
    return nil
}

func buildRecoveryEventXML(ctx context.Context, ts recoveryStore, root model.Task) string {
    list, _ := ts.ListByRoot(ctx, root.ID)
    var sb strings.Builder
    sb.WriteString("<recovery_event>\n")
    fmt.Fprintf(&sb, "<root_task id=\"%s\" status=\"%s\"/>\n", root.ID, root.Status)
    sb.WriteString("<subtasks>\n")
    for _, t := range list {
        if t.ID == root.ID { continue }
        fmt.Fprintf(&sb, "  <subtask id=\"%s\" worker=\"%s\" status=\"%s\"/>\n", t.ID, t.WorkerID, t.Status)
    }
    sb.WriteString("</subtasks>\n</recovery_event>\n")
    return sb.String()
}
```

Add `ListWaitingGroupRoots` to `TaskStore`:

```go
// ListWaitingGroupRoots returns tasks where agent_kind='group' and status IN
// ('waiting_subtasks','running'). Used at startup to recover ongoing group tasks.
func (s *TaskStore) ListWaitingGroupRoots(ctx context.Context) ([]model.Task, error) {
    rows, err := s.db.QueryContext(ctx, `
        SELECT id, message_id, worker_id, instruction, type, status,
               scheduled_at, cron_expr, next_run_at, execution_id,
               parent_task_id, root_task_id, agent_kind,
               created_at, updated_at
        FROM bee_tasks
        WHERE agent_kind = 'group'
          AND status IN ('running','waiting_subtasks')
          AND parent_task_id = ''`)
    if err != nil { return nil, fmt.Errorf("list waiting group roots: %w", err) }
    defer rows.Close()
    return scanTasks(rows)
}
```

Add `SessionKeyForAgent` to `SessionStore` (or use what already exists — there's already `GetSessionContextForEngine` keyed by `(session_key, agent_id, engine)`; the recovery side needs the *reverse* lookup). If the table doesn't index by agent_id, add an index:

Append migration 40:
```go
{
    version: 40,
    name:    "create_index_session_contexts_agent_engine",
    sql:     `CREATE INDEX IF NOT EXISTS idx_session_contexts_agent_engine ON bee_session_contexts(agent_id, engine)`,
},
```

Add the lookup method:
```go
func (s *SessionStore) SessionKeyForAgent(ctx context.Context, agentID, engine string) (string, string, bool, error) {
    row := s.db.QueryRowContext(ctx,
        `SELECT session_key, session_id FROM bee_session_contexts WHERE agent_id = ? AND engine = ? LIMIT 1`,
        agentID, engine)
    var key, sid string
    if err := row.Scan(&key, &sid); err != nil {
        if errors.Is(err, sql.ErrNoRows) { return "", "", false, nil }
        return "", "", false, err
    }
    return key, sid, true, nil
}
```

- [ ] **Step 19.3: Run — expect PASS**

```bash
go test ./internal/domain/task/ -run TestRecoverGroupTasks -v
```

- [ ] **Step 19.4: Commit**

```bash
git add internal/domain/task/recovery.go internal/domain/task/recovery_test.go internal/infra/store/task_store.go internal/infra/store/session_store.go internal/infra/store/db.go
git commit -m "feat(recovery): add RecoverGroupTasks for crash resumption"
```

---

# Phase 6 — Bee feeder entry point

## Task 20: Direct dispatch by group name

**Files:**
- Modify: `internal/domain/bee/feeder.go`
- Modify: `internal/domain/bee/feeder_test.go`

- [ ] **Step 20.1: Failing test**

```go
func TestTryDirectDispatch_RoutesGroupName(t *testing.T) {
    f := newFeederWithGroup(t, "data-team", "group-id-xyz")
    msg := store.ClaimedMessage{ID: "m1", SessionKey: "s1", Content: "@data-team please fetch"}
    if !f.tryDirectDispatch(context.Background(), []store.ClaimedMessage{msg}) {
        t.Fatal("expected direct dispatch to succeed for @data-team")
    }
    tasks, _ := f.taskStore.List(context.Background(), store.TaskFilter{MessageID: "m1"})
    if len(tasks) != 1 || tasks[0].WorkerID != "group-id-xyz" || tasks[0].AgentKind != model.AgentKindGroup {
        t.Errorf("expected one group task, got %+v", tasks)
    }
}
```

- [ ] **Step 20.2: Implement**

Add a `groupLookup *store.GroupStore` field on `Feeder`, plus an option:
```go
func WithGroupDispatch(gs *store.GroupStore) Option {
    return func(f *Feeder) { f.groupLookup = gs }
}
```

Modify `tryDirectDispatch` to consult both worker and group stores. After the existing `f.workerLookup.GetByName(workerName)` block:

```go
if f.groupLookup != nil {
    if g, err := f.groupLookup.GetByName(workerName); err == nil {
        _, err = f.taskStore.Create(ctx, model.Task{
            MessageID:   primary.ID,
            WorkerID:    g.ID,
            Instruction: instruction,
            Type:        model.TaskTypeImmediate,
            Status:      model.TaskStatusPending,
            AgentKind:   model.AgentKindGroup,
        })
        if err != nil {
            log.Error("direct: create group task", zap.Error(err))
            return false
        }
        log.Info("direct: dispatched task to group via scheduler",
            zap.String("name", workerName), zap.String("groupID", g.ID))
        if err := f.msgStore.MarkBeeProcessed(ctx, messageIDs(msgs)); err != nil {
            log.Error("direct: mark bee_processed", zap.Error(err))
        }
        return true
    }
}
```

(Place this **before** the `return false` at the bottom; if worker lookup didn't find a match, fall through to group lookup.)

- [ ] **Step 20.3: Run — expect PASS**

```bash
go test ./internal/domain/bee/ -run TestTryDirectDispatch_RoutesGroupName -v
```

- [ ] **Step 20.4: Commit**

```bash
git add internal/domain/bee/feeder.go internal/domain/bee/feeder_test.go
git commit -m "feat(bee): route @groupName direct dispatch to Group as a root task"
```

---

# Phase 7 — Wiring + E2E

## Task 21: app.go wiring

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 21.1: Construct GroupStore + group.Manager + dispatcher hooks**

In `app.go`, after the `WorkerStore` and `worker.Manager` are constructed:

```go
groupStore := store.NewGroupStore(db)
groupManager := group.NewManager(
    cfg.Groups.BaseDir, // add this to config; default to filepath.Join(cfg.DataDir, "groups")
    groupStore, workerStore, taskStore,
    engines, engineCfg, cfg.Bee.Platforms.BotNames(),
)
```

Pass `groupStore` and `groupManager` into the route deps struct, and wire `WithGroupLookup(groupStore)` and `WithTaskQuerier(taskStore)` to the `task.New(...)` call:

```go
dispatcher := task.New(
    workerManager, taskStore, sessionStore, executionStore, taskInCh, engineCfg,
    task.WithFailureNotifier(failureNotifier),
    task.WithWorkerLookup(workerStore),
    task.WithGroupLookup(groupStore),
    task.WithTaskQuerier(taskStore),
)
```

Pass `WithGroupDispatch(groupStore)` to the feeder constructor:
```go
feeder := bee.NewFeeder(..., bee.WithFailureNotifier(...), bee.WithWorkerDispatch(workerStore), bee.WithGroupDispatch(groupStore))
```

After `dispatcher.Run` is started, fire recovery in a goroutine:
```go
go func() {
    if err := task.RecoverGroupTasks(context.Background(), taskStore, sessionStore, taskInCh, engineCfg.Get()); err != nil {
        log.Error("recover group tasks", zap.Error(err))
    }
}()
```

- [ ] **Step 21.2: Verify build**

```bash
go build ./...
```

- [ ] **Step 21.3: Commit**

```bash
git add internal/app/app.go internal/infra/config/...
git commit -m "feat(app): wire GroupStore, group.Manager, dispatcher group hooks, and recovery"
```

---

## Task 22: End-to-end test

**Files:**
- Create: `internal/domain/task/e2e_group_test.go`

- [ ] **Step 22.1: Write the E2E test**

This test uses an in-memory DB + a scripted fake engine to drive a complete Group lifecycle.

```go
package task

import (
    "context"
    "encoding/json"
    "testing"
    "time"

    "github.com/theopenbee/openbee/internal/domain/bee"
    "github.com/theopenbee/openbee/internal/domain/group"
    "github.com/theopenbee/openbee/internal/infra/model"
    "github.com/theopenbee/openbee/internal/infra/store"
)

// TestE2E_GroupHappyPath drives a full group execution by scripting a fake engine.
func TestE2E_GroupHappyPath(t *testing.T) {
    db, err := store.InitDB(t.TempDir() + "/test.db")
    if err != nil { t.Fatal(err) }
    defer db.Close()

    ws := store.NewWorkerStore(db)
    gs := store.NewGroupStore(db)
    ts := store.NewTaskStore(db)
    ms := store.NewMessageStore(db)
    ss := store.NewSessionStore(db)
    es := store.NewExecutionStore(db)
    out := store.NewOutboundMessageStore(db)

    // Two member workers + one group with both as members.
    w1, _ := ws.Create(model.Worker{Name: "alice", WorkDir: t.TempDir()})
    w2, _ := ws.Create(model.Worker{Name: "bob",   WorkDir: t.TempDir()})
    gm := group.NewManager(t.TempDir(), gs, ws, ts, nil, nil, nil)
    g, _ := gm.CreateGroup(group.CreateGroupParams{Name: "data-team", Description: "ETL", Constraints: ""})
    _ = gm.AddMember(g.ID, w1.ID)
    _ = gm.AddMember(g.ID, w2.ID)

    // Insert a platform message that mentions @data-team.
    msgID := seedPlatformMessage(t, ms, "session-1", "@data-team please fetch X")

    // Build a scripted engine: when the agent ID is the group, run a script that calls dispatch-subtask × 2 + suspend.
    fakeEngine := newScriptedEngine(map[string][]string{
        g.ID: {
            "openbee ctl message send --message-id " + msgID + " --stdin <<EOF\nstarting...\nEOF",
            "openbee ctl task dispatch-subtask --parent-task-id <ROOT> --worker-id " + w1.ID + " --stdin <<EOF\npart 1\nEOF",
            "openbee ctl task dispatch-subtask --parent-task-id <ROOT> --worker-id " + w2.ID + " --stdin <<EOF\npart 2\nEOF",
            "openbee ctl task suspend --task-id <ROOT>",
        },
        w1.ID: {"openbee ctl message send --message-id <SUB_MSG> --stdin <<EOF\npart 1 done\nEOF"},
        w2.ID: {"openbee ctl message send --message-id <SUB_MSG> --stdin <<EOF\npart 2 done\nEOF"},
    })

    // Wire the dispatcher with the fake engine and recovery.
    inCh := make(chan DispatchTask, 16)
    dispatcher := New(newFakeExecMgrWithEngine(fakeEngine), ts, ss, es, inCh, nil,
        WithGroupLookup(gs), WithTaskQuerier(ts))
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    go dispatcher.Run(ctx)

    // Bee feeder consumes the message and creates the root group task.
    feeder := bee.NewFeeder(ms, ts, ss, es, fakeEngine, t.TempDir(), defaultBeeCfg(),
        nil, bee.WithGroupDispatch(gs))
    go feeder.Run(ctx)

    // Wait for the user to receive 3 outbound messages: opening + 2 progress + final.
    if err := waitFor(time.Second*5, func() bool {
        list, _ := out.ListByMessageID(msgID)
        return len(list) >= 3
    }); err != nil {
        t.Fatalf("expected ≥3 outbound messages, timed out: %v", err)
    }

    // All tasks should be completed.
    list, _ := ts.ListByRoot(ctx, /* root task id */ findRootTaskID(t, ts, msgID))
    for _, tk := range list {
        if tk.Status != model.TaskStatusCompleted {
            t.Errorf("task %s not completed: %s", tk.ID, tk.Status)
        }
    }
}
```

The `scriptedEngine` is a fake that, when `Run` is called, executes each script line in sequence as if they were CLI calls — i.e., it directly calls the corresponding `internal/api` handler functions (via http.NewRecorder against the test gin router). Build it as a small helper inside this test file. The placeholders `<ROOT>` / `<SUB_MSG>` are substituted at run time from context — the helper resolves them by looking up the current task ID being executed.

> If this scripted-engine pattern is too heavy for a single test file, factor it into `internal/ai/fake/scripted_engine.go` and reuse from this and other tests.

- [ ] **Step 22.2: Run — expect PASS**

```bash
go test ./internal/domain/task/ -run TestE2E_GroupHappyPath -v
```

- [ ] **Step 22.3: Commit**

```bash
git add internal/domain/task/e2e_group_test.go
git commit -m "test(group): end-to-end happy path with scripted fake engine"
```

---

# Final integration verification

## Task 23: Full build + full test sweep

- [ ] **Step 23.1: Run the full test suite**

```bash
go test ./...
```
Expected: all tests pass. If any pre-existing tests break, debug — typically because of the `bee_tasks` table recreation in migration 39 dropping a foreign key relationship that some other test depended on. Adjust the affected test rather than the migration unless the migration itself is wrong.

- [ ] **Step 23.2: Run `go vet` and `gofmt`**

```bash
go vet ./...
gofmt -l .
```
Expected: no vet errors; no gofmt diffs.

- [ ] **Step 23.3: Smoke-test the CLI manually**

In a separate shell:
```bash
go run ./cmd/openbee server -d
openbee ctl group create --name demo --description "smoke test"
openbee ctl group list
openbee ctl group member add --group <id-from-list> --worker <existing-worker-id>
openbee ctl group get <id>
openbee ctl group delete <id>
```
Expected: each command returns success without panics.

- [ ] **Step 23.4: Final commit (if any unstaged fixes)**

```bash
git status
# If anything is unstaged from the verification:
git add <files>
git commit -m "fix: <details from verification step>"
```

---

# Self-Review Checklist (run by author after writing the plan)

**1. Spec coverage** — every spec section maps to at least one task:
- §3 Architecture / module list → Tasks 2–13, 14, 16, 19, 20, 21
- §4 Data Model → Tasks 1, 2, 5
- §5 Runtime Sequence (CLI commands + sequence + sessionKey) → Tasks 8–13, 14, 15, 17
- §6 Error Handling → Tasks 15 (worker fail path), 17 (phantom suspend), 18 (cancel cascade), 19 (recovery), 6 (group delete validation)
- §7 Testing Strategy → Tasks 3, 4, 5, 6, 7, 9, 10, 14, 15, 16, 17, 18, 19, 20, 22

**2. Placeholder scan** — none of the disallowed phrases appear; every step shows code or exact commands.

**3. Type consistency** — `agent_kind`/`AgentKind*`, `parent_task_id`/`ParentTaskID`, `root_task_id`/`RootTaskID`, `TaskStatusWaitingSubtasks`, `BuildPersona`, `MemberBrief`, `GroupBrief`, `GroupLookup`, `TaskQuerier`, `subtaskEventCh`, `RecoverGroupTasks` are used consistently across tasks.

**4. Order dependencies** — each task only references identifiers introduced earlier in the plan (model consts → store → manager → handlers → dispatcher → recovery → e2e). Migration 39 correctly comes after 36 because 36 only adds columns; 39 is needed because SQLite cannot ALTER CHECK constraints.
