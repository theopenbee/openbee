# Worker Departments Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add multi-level department hierarchy for organizing Workers in the UI, with many-to-many Worker-Department relationships.

**Architecture:** Two new SQLite tables (`bee_departments`, `bee_worker_departments`) with a new `DepartmentStore` for data access. New API handler for department CRUD and worker-department association. Frontend adds a department tree sidebar to the Workers page.

**Tech Stack:** Go (Gin, database/sql, SQLite), TypeScript (React, React Query, shadcn/ui)

**Spec:** `docs/superpowers/specs/2026-04-05-worker-departments-design.md`

---

### Task 1: Database Migration — Create department tables

**Files:**
- Modify: `internal/infra/store/db.go` (append migrations to the `migrations` slice, currently ends at version 21)

- [ ] **Step 1: Add migration for bee_departments table**

In `internal/infra/store/db.go`, append to the `migrations` slice after the last entry (version 21):

```go
{
    version: 22,
    name:    "create_table_bee_departments",
    sql: `CREATE TABLE IF NOT EXISTS bee_departments (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    parent_id  TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
)`,
},
{
    version: 23,
    name:    "create_index_departments_parent_id",
    sql:     `CREATE INDEX IF NOT EXISTS idx_departments_parent_id ON bee_departments(parent_id)`,
},
{
    version: 24,
    name:    "create_table_bee_worker_departments",
    sql: `CREATE TABLE IF NOT EXISTS bee_worker_departments (
    worker_id     TEXT NOT NULL REFERENCES bee_workers(id),
    department_id TEXT NOT NULL REFERENCES bee_departments(id),
    created_at    INTEGER NOT NULL,
    PRIMARY KEY (worker_id, department_id)
)`,
},
{
    version: 25,
    name:    "create_index_worker_depts_worker",
    sql:     `CREATE INDEX IF NOT EXISTS idx_worker_depts_worker ON bee_worker_departments(worker_id)`,
},
{
    version: 26,
    name:    "create_index_worker_depts_dept",
    sql:     `CREATE INDEX IF NOT EXISTS idx_worker_depts_dept ON bee_worker_departments(department_id)`,
},
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add internal/infra/store/db.go
git commit -m "feat(db): add migrations for departments and worker-departments tables"
```

---

### Task 2: Department Model

**Files:**
- Create: `internal/infra/model/department.go`

- [ ] **Step 1: Create the department model file**

Create `internal/infra/model/department.go`:

```go
package model

// Department represents a department node in a tree hierarchy.
type Department struct {
	ID        string  `json:"id" db:"id"`
	Name      string  `json:"name" db:"name"`
	ParentID  *string `json:"parent_id" db:"parent_id"`
	SortOrder int     `json:"sort_order" db:"sort_order"`
	CreatedAt int64   `json:"created_at" db:"created_at"`
	UpdatedAt int64   `json:"updated_at" db:"updated_at"`
}

// DepartmentTree is a Department with its nested children for tree responses.
type DepartmentTree struct {
	Department
	Children []DepartmentTree `json:"children"`
}

// WorkerDepartment represents the many-to-many link between a Worker and a Department.
type WorkerDepartment struct {
	WorkerID     string `json:"worker_id" db:"worker_id"`
	DepartmentID string `json:"department_id" db:"department_id"`
	CreatedAt    int64  `json:"created_at" db:"created_at"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/infra/model/...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add internal/infra/model/department.go
git commit -m "feat(model): add Department, DepartmentTree, and WorkerDepartment models"
```

---

### Task 3: DepartmentStore — CRUD and Tree

**Files:**
- Create: `internal/infra/store/department_store.go`
- Create: `internal/infra/store/department_store_test.go`

- [ ] **Step 1: Write tests for department CRUD and tree building**

Create `internal/infra/store/department_store_test.go`:

```go
package store

import (
	"testing"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func setupDeptTestDB(t *testing.T) (*DepartmentStore, *WorkerStore) {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewDepartmentStore(db), NewWorkerStore(db)
}

func TestDepartmentStore_Create(t *testing.T) {
	ds, _ := setupDeptTestDB(t)
	d, err := ds.Create(model.Department{Name: "Engineering"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.ID == "" {
		t.Error("expected non-empty ID")
	}
	if d.Name != "Engineering" {
		t.Errorf("expected Engineering, got %s", d.Name)
	}
}

func TestDepartmentStore_GetByID(t *testing.T) {
	ds, _ := setupDeptTestDB(t)
	d, _ := ds.Create(model.Department{Name: "Sales"})
	got, err := ds.GetByID(d.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Sales" {
		t.Errorf("expected Sales, got %s", got.Name)
	}
}

func TestDepartmentStore_Update(t *testing.T) {
	ds, _ := setupDeptTestDB(t)
	d, _ := ds.Create(model.Department{Name: "Old"})
	d.Name = "New"
	updated, err := ds.Update(d)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "New" {
		t.Errorf("expected New, got %s", updated.Name)
	}
}

func TestDepartmentStore_Delete_Empty(t *testing.T) {
	ds, _ := setupDeptTestDB(t)
	d, _ := ds.Create(model.Department{Name: "ToDelete"})
	if err := ds.Delete(d.ID); err != nil {
		t.Fatalf("Delete empty dept: %v", err)
	}
	_, err := ds.GetByID(d.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestDepartmentStore_Delete_HasChildren(t *testing.T) {
	ds, _ := setupDeptTestDB(t)
	parent, _ := ds.Create(model.Department{Name: "Parent"})
	ds.Create(model.Department{Name: "Child", ParentID: &parent.ID})
	err := ds.Delete(parent.ID)
	if err == nil {
		t.Error("expected error deleting department with children")
	}
}

func TestDepartmentStore_Delete_HasWorkers(t *testing.T) {
	ds, ws := setupDeptTestDB(t)
	dept, _ := ds.Create(model.Department{Name: "Dept"})
	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	ds.SetWorkerDepartments(w.ID, []string{dept.ID})
	err := ds.Delete(dept.ID)
	if err == nil {
		t.Error("expected error deleting department with workers")
	}
}

func TestDepartmentStore_BuildTree(t *testing.T) {
	ds, _ := setupDeptTestDB(t)
	root, _ := ds.Create(model.Department{Name: "Root", SortOrder: 0})
	child, _ := ds.Create(model.Department{Name: "Child", ParentID: &root.ID, SortOrder: 0})
	ds.Create(model.Department{Name: "Grandchild", ParentID: &child.ID, SortOrder: 0})

	all, _ := ds.ListAll()
	tree := ds.BuildTree(all)

	if len(tree) != 1 {
		t.Fatalf("expected 1 root, got %d", len(tree))
	}
	if tree[0].Name != "Root" {
		t.Errorf("expected Root, got %s", tree[0].Name)
	}
	if len(tree[0].Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(tree[0].Children))
	}
	if len(tree[0].Children[0].Children) != 1 {
		t.Fatalf("expected 1 grandchild, got %d", len(tree[0].Children[0].Children))
	}
}

func TestDepartmentStore_BuildTree_SortOrder(t *testing.T) {
	ds, _ := setupDeptTestDB(t)
	ds.Create(model.Department{Name: "B", SortOrder: 2})
	ds.Create(model.Department{Name: "A", SortOrder: 1})
	ds.Create(model.Department{Name: "C", SortOrder: 3})

	all, _ := ds.ListAll()
	tree := ds.BuildTree(all)

	if len(tree) != 3 {
		t.Fatalf("expected 3 roots, got %d", len(tree))
	}
	if tree[0].Name != "A" || tree[1].Name != "B" || tree[2].Name != "C" {
		t.Errorf("expected A,B,C order, got %s,%s,%s", tree[0].Name, tree[1].Name, tree[2].Name)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/infra/store/ -run TestDepartmentStore -v`
Expected: compilation errors (DepartmentStore not defined yet)

- [ ] **Step 3: Implement DepartmentStore**

Create `internal/infra/store/department_store.go`:

```go
package store

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type DepartmentStore struct {
	db *sql.DB
}

func NewDepartmentStore(db *sql.DB) *DepartmentStore {
	return &DepartmentStore{db: db}
}

const departmentColumns = `id, name, parent_id, sort_order, created_at, updated_at`

func scanDepartment(scanner interface{ Scan(...any) error }) (model.Department, error) {
	var d model.Department
	err := scanner.Scan(&d.ID, &d.Name, &d.ParentID, &d.SortOrder, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func (s *DepartmentStore) Create(d model.Department) (model.Department, error) {
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	now := time.Now().UnixMilli()
	d.CreatedAt = now
	d.UpdatedAt = now

	if d.ParentID != nil {
		if _, err := s.GetByID(*d.ParentID); err != nil {
			return model.Department{}, fmt.Errorf("parent department not found")
		}
	}

	_, err := s.db.Exec(
		`INSERT INTO bee_departments (id, name, parent_id, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		d.ID, d.Name, d.ParentID, d.SortOrder, d.CreatedAt, d.UpdatedAt,
	)
	if err != nil {
		return model.Department{}, fmt.Errorf("insert department: %w", err)
	}
	return d, nil
}

func (s *DepartmentStore) GetByID(id string) (model.Department, error) {
	row := s.db.QueryRow(`SELECT `+departmentColumns+` FROM bee_departments WHERE id = ?`, id)
	d, err := scanDepartment(row)
	if err != nil {
		return model.Department{}, fmt.Errorf("get department: %w", err)
	}
	return d, nil
}

func (s *DepartmentStore) ListAll() ([]model.Department, error) {
	rows, err := s.db.Query(`SELECT ` + departmentColumns + ` FROM bee_departments ORDER BY sort_order, created_at`)
	if err != nil {
		return nil, fmt.Errorf("list departments: %w", err)
	}
	defer rows.Close()

	var departments []model.Department
	for rows.Next() {
		d, err := scanDepartment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan department: %w", err)
		}
		departments = append(departments, d)
	}
	return departments, rows.Err()
}

func (s *DepartmentStore) Update(d model.Department) (model.Department, error) {
	d.UpdatedAt = time.Now().UnixMilli()
	_, err := s.db.Exec(
		`UPDATE bee_departments SET name=?, parent_id=?, sort_order=?, updated_at=? WHERE id=?`,
		d.Name, d.ParentID, d.SortOrder, d.UpdatedAt, d.ID,
	)
	if err != nil {
		return model.Department{}, fmt.Errorf("update department: %w", err)
	}
	return d, nil
}

func (s *DepartmentStore) Delete(id string) error {
	hasChildren, err := s.HasChildren(id)
	if err != nil {
		return err
	}
	if hasChildren {
		return fmt.Errorf("department is not empty: has sub-departments")
	}

	hasWorkers, err := s.HasWorkers(id)
	if err != nil {
		return err
	}
	if hasWorkers {
		return fmt.Errorf("department is not empty: has associated workers")
	}

	_, err = s.db.Exec(`DELETE FROM bee_departments WHERE id=?`, id)
	return err
}

func (s *DepartmentStore) HasChildren(id string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM bee_departments WHERE parent_id = ?`, id).Scan(&count)
	return count > 0, err
}

func (s *DepartmentStore) HasWorkers(id string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM bee_worker_departments WHERE department_id = ?`, id).Scan(&count)
	return count > 0, err
}

// CheckCircularReference returns an error if setting parentID as the parent of
// departmentID would create a cycle.
func (s *DepartmentStore) CheckCircularReference(departmentID, parentID string) error {
	current := parentID
	for current != "" {
		if current == departmentID {
			return fmt.Errorf("circular reference detected")
		}
		d, err := s.GetByID(current)
		if err != nil {
			return nil // parent chain broken, no cycle
		}
		if d.ParentID == nil {
			break
		}
		current = *d.ParentID
	}
	return nil
}

// BuildTree assembles a flat list of departments into a tree structure.
func (s *DepartmentStore) BuildTree(depts []model.Department) []model.DepartmentTree {
	nodeMap := make(map[string]*model.DepartmentTree, len(depts))
	for _, d := range depts {
		nodeMap[d.ID] = &model.DepartmentTree{Department: d, Children: []model.DepartmentTree{}}
	}

	var roots []*model.DepartmentTree
	for _, d := range depts {
		node := nodeMap[d.ID]
		if d.ParentID != nil {
			if parent, ok := nodeMap[*d.ParentID]; ok {
				parent.Children = append(parent.Children, *node)
				continue
			}
		}
		roots = append(roots, node)
	}

	result := make([]model.DepartmentTree, 0, len(roots))
	for _, r := range roots {
		result = append(result, *r)
	}

	// Sort roots and all children by sort_order, then created_at
	sortTreeSlice(result)
	return result
}

func sortTreeSlice(nodes []model.DepartmentTree) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].SortOrder != nodes[j].SortOrder {
			return nodes[i].SortOrder < nodes[j].SortOrder
		}
		return nodes[i].CreatedAt < nodes[j].CreatedAt
	})
	for i := range nodes {
		sortTreeSlice(nodes[i].Children)
	}
}

// SetWorkerDepartments replaces all department associations for a worker.
func (s *DepartmentStore) SetWorkerDepartments(workerID string, deptIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM bee_worker_departments WHERE worker_id = ?`, workerID); err != nil {
		tx.Rollback()
		return fmt.Errorf("clear worker departments: %w", err)
	}

	now := time.Now().UnixMilli()
	for _, deptID := range deptIDs {
		if _, err := tx.Exec(
			`INSERT INTO bee_worker_departments (worker_id, department_id, created_at) VALUES (?, ?, ?)`,
			workerID, deptID, now,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert worker department: %w", err)
		}
	}

	return tx.Commit()
}

// GetWorkerDepartments returns all departments a worker belongs to.
func (s *DepartmentStore) GetWorkerDepartments(workerID string) ([]model.Department, error) {
	rows, err := s.db.Query(
		`SELECT d.`+departmentColumns+` FROM bee_departments d
		 INNER JOIN bee_worker_departments wd ON d.id = wd.department_id
		 WHERE wd.worker_id = ?
		 ORDER BY d.sort_order, d.created_at`,
		workerID,
	)
	if err != nil {
		return nil, fmt.Errorf("get worker departments: %w", err)
	}
	defer rows.Close()

	var departments []model.Department
	for rows.Next() {
		d, err := scanDepartment(rows)
		if err != nil {
			return nil, err
		}
		departments = append(departments, d)
	}
	return departments, rows.Err()
}

// GetDepartmentWorkerIDs returns the IDs of workers directly associated with a department.
func (s *DepartmentStore) GetDepartmentWorkerIDs(deptID string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT worker_id FROM bee_worker_departments WHERE department_id = ?`, deptID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteWorkerDepartments removes all department associations for a worker.
// Called when a worker is deleted.
func (s *DepartmentStore) DeleteWorkerDepartments(workerID string) error {
	_, err := s.db.Exec(`DELETE FROM bee_worker_departments WHERE worker_id = ?`, workerID)
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/infra/store/ -run TestDepartmentStore -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/department_store.go internal/infra/store/department_store_test.go
git commit -m "feat(store): add DepartmentStore with CRUD, tree building, and worker associations"
```

---

### Task 4: Worker-Department Association Tests

**Files:**
- Modify: `internal/infra/store/department_store_test.go`

- [ ] **Step 1: Add tests for worker-department association operations**

Append to `internal/infra/store/department_store_test.go`:

```go
func TestDepartmentStore_SetWorkerDepartments(t *testing.T) {
	ds, ws := setupDeptTestDB(t)
	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	d1, _ := ds.Create(model.Department{Name: "Dept1"})
	d2, _ := ds.Create(model.Department{Name: "Dept2"})

	// Set initial departments
	if err := ds.SetWorkerDepartments(w.ID, []string{d1.ID, d2.ID}); err != nil {
		t.Fatalf("SetWorkerDepartments: %v", err)
	}
	depts, _ := ds.GetWorkerDepartments(w.ID)
	if len(depts) != 2 {
		t.Errorf("expected 2 departments, got %d", len(depts))
	}

	// Replace with just one
	if err := ds.SetWorkerDepartments(w.ID, []string{d1.ID}); err != nil {
		t.Fatalf("SetWorkerDepartments replace: %v", err)
	}
	depts, _ = ds.GetWorkerDepartments(w.ID)
	if len(depts) != 1 {
		t.Errorf("expected 1 department, got %d", len(depts))
	}

	// Clear all
	if err := ds.SetWorkerDepartments(w.ID, []string{}); err != nil {
		t.Fatalf("SetWorkerDepartments clear: %v", err)
	}
	depts, _ = ds.GetWorkerDepartments(w.ID)
	if len(depts) != 0 {
		t.Errorf("expected 0 departments, got %d", len(depts))
	}
}

func TestDepartmentStore_GetDepartmentWorkerIDs(t *testing.T) {
	ds, ws := setupDeptTestDB(t)
	dept, _ := ds.Create(model.Department{Name: "Dept"})
	w1, _ := ws.Create(model.Worker{Name: "Bot1", WorkDir: "/tmp/b1"})
	w2, _ := ws.Create(model.Worker{Name: "Bot2", WorkDir: "/tmp/b2"})

	ds.SetWorkerDepartments(w1.ID, []string{dept.ID})
	ds.SetWorkerDepartments(w2.ID, []string{dept.ID})

	ids, err := ds.GetDepartmentWorkerIDs(dept.ID)
	if err != nil {
		t.Fatalf("GetDepartmentWorkerIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 worker IDs, got %d", len(ids))
	}
}

func TestDepartmentStore_DeleteWorkerDepartments(t *testing.T) {
	ds, ws := setupDeptTestDB(t)
	dept, _ := ds.Create(model.Department{Name: "Dept"})
	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	ds.SetWorkerDepartments(w.ID, []string{dept.ID})

	if err := ds.DeleteWorkerDepartments(w.ID); err != nil {
		t.Fatalf("DeleteWorkerDepartments: %v", err)
	}
	depts, _ := ds.GetWorkerDepartments(w.ID)
	if len(depts) != 0 {
		t.Errorf("expected 0 departments after cleanup, got %d", len(depts))
	}
}

func TestDepartmentStore_CheckCircularReference(t *testing.T) {
	ds, _ := setupDeptTestDB(t)
	a, _ := ds.Create(model.Department{Name: "A"})
	b, _ := ds.Create(model.Department{Name: "B", ParentID: &a.ID})
	c, _ := ds.Create(model.Department{Name: "C", ParentID: &b.ID})

	// Moving A under C would create A -> B -> C -> A cycle
	err := ds.CheckCircularReference(a.ID, c.ID)
	if err == nil {
		t.Error("expected circular reference error")
	}

	// Moving C under A is fine (already the case via B)
	err = ds.CheckCircularReference(c.ID, a.ID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 2: Run all department store tests**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/infra/store/ -run TestDepartmentStore -v`
Expected: all tests PASS

- [ ] **Step 3: Commit**

```bash
git add internal/infra/store/department_store_test.go
git commit -m "test(store): add tests for worker-department associations and circular reference detection"
```

---

### Task 5: Department API Handler

**Files:**
- Create: `internal/api/department_handler.go`
- Modify: `internal/api/router.go` (add `DepartmentStore` to `ServerParams`, register routes)

- [ ] **Step 1: Create department handler**

Create `internal/api/department_handler.go`:

```go
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type createDepartmentRequest struct {
	Name      string  `json:"name" binding:"required"`
	ParentID  *string `json:"parent_id"`
	SortOrder int     `json:"sort_order"`
}

func (s *Server) createDepartment(c *gin.Context) {
	var req createDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	d, err := s.DepartmentStore.Create(model.Department{
		Name:      req.Name,
		ParentID:  req.ParentID,
		SortOrder: req.SortOrder,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, d)
}

func (s *Server) listDepartments(c *gin.Context) {
	depts, err := s.DepartmentStore.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tree := s.DepartmentStore.BuildTree(depts)
	c.JSON(http.StatusOK, tree)
}

func (s *Server) getDepartment(c *gin.Context) {
	d, err := s.DepartmentStore.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "department not found"})
		return
	}
	c.JSON(http.StatusOK, d)
}

func (s *Server) updateDepartment(c *gin.Context) {
	d, err := s.DepartmentStore.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "department not found"})
		return
	}

	var req struct {
		Name      *string `json:"name"`
		ParentID  *string `json:"parent_id"`
		SortOrder *int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != nil {
		d.Name = *req.Name
	}
	if req.ParentID != nil {
		if err := s.DepartmentStore.CheckCircularReference(d.ID, *req.ParentID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		d.ParentID = req.ParentID
	}
	if req.SortOrder != nil {
		d.SortOrder = *req.SortOrder
	}

	updated, err := s.DepartmentStore.Update(d)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *Server) deleteDepartment(c *gin.Context) {
	if err := s.DepartmentStore.Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

type setWorkerDepartmentsRequest struct {
	DepartmentIDs []string `json:"department_ids" binding:"required"`
}

func (s *Server) setWorkerDepartments(c *gin.Context) {
	workerID := c.Param("id")
	if _, err := s.WorkerStore.GetByID(workerID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}

	var req setWorkerDepartmentsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate all department IDs exist
	for _, id := range req.DepartmentIDs {
		if _, err := s.DepartmentStore.GetByID(id); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "department not found: " + id})
			return
		}
	}

	if err := s.DepartmentStore.SetWorkerDepartments(workerID, req.DepartmentIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"department_ids": req.DepartmentIDs})
}

func (s *Server) getWorkerDepartments(c *gin.Context) {
	workerID := c.Param("id")
	if _, err := s.WorkerStore.GetByID(workerID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}

	depts, err := s.DepartmentStore.GetWorkerDepartments(workerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if depts == nil {
		depts = []model.Department{}
	}
	c.JSON(http.StatusOK, depts)
}

func (s *Server) getDepartmentWorkers(c *gin.Context) {
	deptID := c.Param("id")
	if _, err := s.DepartmentStore.GetByID(deptID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "department not found"})
		return
	}

	workerIDs, err := s.DepartmentStore.GetDepartmentWorkerIDs(deptID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var workers []model.Worker
	for _, wid := range workerIDs {
		w, err := s.WorkerStore.GetByID(wid)
		if err != nil {
			continue
		}
		workers = append(workers, w)
	}
	if workers == nil {
		workers = []model.Worker{}
	}
	c.JSON(http.StatusOK, workers)
}
```

Note: This file needs to import the model package. Add this import at the top:

```go
import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
)
```

- [ ] **Step 2: Add DepartmentStore to ServerParams and register routes**

In `internal/api/router.go`, add `DepartmentStore` to `ServerParams`:

```go
// Add to the import block (if not already present):
// "github.com/theopenbee/openbee/internal/infra/store"
// (already imported)

// Add to ServerParams struct:
DepartmentStore  *store.DepartmentStore
```

Add route registration call in `setupRoutes()`, inside the `api.Use(s.JWTMiddleware)` block, after `s.registerTaskRoutes(api)`:

```go
s.registerDepartmentRoutes(api)
```

Add the route registration method:

```go
func (s *Server) registerDepartmentRoutes(api *gin.RouterGroup) {
	api.POST("/departments", s.createDepartment)
	api.GET("/departments", s.listDepartments)
	api.GET("/departments/:id", s.getDepartment)
	api.PUT("/departments/:id", s.updateDepartment)
	api.DELETE("/departments/:id", s.deleteDepartment)
	api.PUT("/workers/:id/departments", s.setWorkerDepartments)
	api.GET("/workers/:id/departments", s.getWorkerDepartments)
	api.GET("/departments/:id/workers", s.getDepartmentWorkers)
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./...`
Expected: compilation error — `DepartmentStore` not yet wired in `app.go`. That's fine for now, we fix it in the next task.

- [ ] **Step 4: Commit**

```bash
git add internal/api/department_handler.go internal/api/router.go
git commit -m "feat(api): add department CRUD and worker-department association handlers"
```

---

### Task 6: Wire DepartmentStore into App

**Files:**
- Modify: `internal/app/app.go` (add DepartmentStore to `appStores`, wire into server)

- [ ] **Step 1: Add DepartmentStore to appStores and buildStores**

In `internal/app/app.go`, add field to `appStores` struct:

```go
departmentStore   *store.DepartmentStore
```

In `buildStores` function, add to the returned `appStores`:

```go
departmentStore:   store.NewDepartmentStore(db),
```

- [ ] **Step 2: Pass DepartmentStore to API server**

In `buildAPIServer` function, add to the `api.ServerParams` struct literal:

```go
DepartmentStore:  s.departmentStore,
```

- [ ] **Step 3: Verify it compiles and all tests pass**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./... && go test ./...`
Expected: build succeeds, all tests pass

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): wire DepartmentStore into application and API server"
```

---

### Task 7: Modify Worker API to Include Departments

**Files:**
- Modify: `internal/api/worker_handler.go`

- [ ] **Step 1: Add departments to worker list response**

In `internal/api/worker_handler.go`, modify `listWorkers` to enrich workers with department info. Replace the current `listWorkers` function:

```go
type workerResponse struct {
	model.Worker
	Departments []departmentBrief `json:"departments"`
}

type departmentBrief struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Server) listWorkers(c *gin.Context) {
	workers, err := s.WorkerStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Filter by department_id if provided
	deptID := c.Query("department_id")

	var result []workerResponse
	for _, w := range workers {
		depts, _ := s.DepartmentStore.GetWorkerDepartments(w.ID)
		if deptID != "" {
			found := false
			for _, d := range depts {
				if d.ID == deptID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		briefs := make([]departmentBrief, 0, len(depts))
		for _, d := range depts {
			briefs = append(briefs, departmentBrief{ID: d.ID, Name: d.Name})
		}
		result = append(result, workerResponse{Worker: w, Departments: briefs})
	}
	if result == nil {
		result = []workerResponse{}
	}
	c.JSON(http.StatusOK, result)
}
```

Add model import if not present:

```go
"github.com/theopenbee/openbee/internal/infra/model"
```

- [ ] **Step 2: Add departments to getWorker response**

Replace the current `getWorker` function:

```go
func (s *Server) getWorker(c *gin.Context) {
	w, err := s.WorkerStore.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}
	depts, _ := s.DepartmentStore.GetWorkerDepartments(w.ID)
	briefs := make([]departmentBrief, 0, len(depts))
	for _, d := range depts {
		briefs = append(briefs, departmentBrief{ID: d.ID, Name: d.Name})
	}
	c.JSON(http.StatusOK, workerResponse{Worker: w, Departments: briefs})
}
```

- [ ] **Step 3: Clean up worker departments on delete**

In the `deleteWorker` function, add department cleanup before the existing `s.Manager.DeleteWorker` call. Insert this line before calling `s.Manager.DeleteWorker`:

```go
s.DepartmentStore.DeleteWorkerDepartments(id)
```

- [ ] **Step 4: Verify it compiles**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./...`
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add internal/api/worker_handler.go
git commit -m "feat(api): enrich worker responses with department info and add department_id filter"
```

---

### Task 8: Frontend Types and API Client

**Files:**
- Modify: `web/src/lib/types.ts`
- Modify: `web/src/lib/api.ts`

- [ ] **Step 1: Add department types**

Append to `web/src/lib/types.ts`:

```typescript
export interface Department {
  id: string
  name: string
  parent_id: string | null
  sort_order: number
  created_at: number
  updated_at: number
}

export interface DepartmentTree extends Department {
  children: DepartmentTree[]
}

export interface DepartmentBrief {
  id: string
  name: string
}
```

Update the `Worker` interface to add departments:

```typescript
export interface Worker {
  id: string
  name: string
  description: string
  memory: string
  work_dir: string
  status: WorkerStatus
  departments?: DepartmentBrief[]
  created_at: number
  updated_at: number
}
```

- [ ] **Step 2: Add department API methods**

In `web/src/lib/api.ts`, add the import for the new types at the top:

```typescript
import type { Worker, WorkerExecution, PaginatedResponse, LocalChatSession, ChatMessage, Task, Department, DepartmentTree } from "./types"
```

Add `departments` section to the `api` object (after `tasks`):

```typescript
departments: {
  list: async () => {
    const tree = await fetchAPI<DepartmentTree[] | null>("/departments")
    return Array.isArray(tree) ? tree : []
  },
  get: (id: string) => fetchAPI<Department>(`/departments/${id}`),
  create: (data: { name: string; parent_id?: string | null; sort_order?: number }) =>
    fetchAPI<Department>("/departments", { method: "POST", body: JSON.stringify(data) }),
  update: (id: string, data: { name?: string; parent_id?: string | null; sort_order?: number }) =>
    fetchAPI<Department>(`/departments/${id}`, { method: "PUT", body: JSON.stringify(data) }),
  delete: (id: string) =>
    fetchAPI(`/departments/${id}`, { method: "DELETE" }),
  workers: (id: string) => fetchAPI<Worker[]>(`/departments/${id}/workers`),
},
```

Update the `workers.list` method to support department_id filter:

```typescript
list: async (departmentId?: string) => {
  const qs = departmentId ? `?department_id=${departmentId}` : ""
  const workers = await fetchAPI<Worker[] | null>(`/workers${qs}`)
  return Array.isArray(workers) ? workers : []
},
```

Add worker department methods inside the `workers` object:

```typescript
getDepartments: (id: string) => fetchAPI<Department[]>(`/workers/${id}/departments`),
setDepartments: (id: string, departmentIds: string[]) =>
  fetchAPI(`/workers/${id}/departments`, {
    method: "PUT",
    body: JSON.stringify({ department_ids: departmentIds }),
  }),
```

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/types.ts web/src/lib/api.ts
git commit -m "feat(web): add department types and API client methods"
```

---

### Task 9: Frontend React Query Hooks for Departments

**Files:**
- Create: `web/src/hooks/use-departments.ts`
- Modify: `web/src/hooks/use-workers.ts`

- [ ] **Step 1: Create department hooks**

Create `web/src/hooks/use-departments.ts`:

```typescript
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api"

export function useDepartments() {
  return useQuery({
    queryKey: ["departments"],
    queryFn: api.departments.list,
  })
}

export function useCreateDepartment() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: { name: string; parent_id?: string | null; sort_order?: number }) =>
      api.departments.create(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["departments"] })
    },
  })
}

export function useUpdateDepartment() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: { name?: string; parent_id?: string | null; sort_order?: number } }) =>
      api.departments.update(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["departments"] })
    },
  })
}

export function useDeleteDepartment() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.departments.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["departments"] })
    },
  })
}

export function useSetWorkerDepartments() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ workerId, departmentIds }: { workerId: string; departmentIds: string[] }) =>
      api.workers.setDepartments(workerId, departmentIds),
    onSuccess: (_, { workerId }) => {
      queryClient.invalidateQueries({ queryKey: ["workers"] })
      queryClient.invalidateQueries({ queryKey: ["workers", workerId] })
      queryClient.invalidateQueries({ queryKey: ["departments"] })
    },
  })
}
```

- [ ] **Step 2: Update useWorkers hook to accept department filter**

In `web/src/hooks/use-workers.ts`, update the `useWorkers` function:

```typescript
export function useWorkers(departmentId?: string) {
  return useQuery({
    queryKey: ["workers", { departmentId }],
    queryFn: () => api.workers.list(departmentId),
  })
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/hooks/use-departments.ts web/src/hooks/use-workers.ts
git commit -m "feat(web): add department React Query hooks and department filter for useWorkers"
```

---

### Task 10: Department Tree Sidebar Component

**Files:**
- Create: `web/src/components/department-tree.tsx`

- [ ] **Step 1: Create the department tree component**

Create `web/src/components/department-tree.tsx`:

```tsx
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { ChevronRightIcon, FolderIcon, FolderOpenIcon, UsersIcon, InboxIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import type { DepartmentTree as DepartmentTreeType } from "@/lib/types"

interface DepartmentTreeProps {
  departments: DepartmentTreeType[]
  selectedId: string | null // null = "all", "ungrouped" = ungrouped
  onSelect: (id: string | null) => void
  onManage: () => void
}

export function DepartmentTreeSidebar({ departments, selectedId, onSelect, onManage }: DepartmentTreeProps) {
  const { t } = useTranslation()

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto py-2 space-y-0.5">
        {/* All Workers */}
        <button
          onClick={() => onSelect(null)}
          className={cn(
            "w-full flex items-center gap-2 px-3 py-1.5 text-sm rounded-md transition-colors",
            selectedId === null
              ? "bg-primary/10 text-primary font-medium"
              : "text-muted-foreground hover:bg-muted"
          )}
        >
          <UsersIcon className="size-4 shrink-0" />
          {t("departments.allWorkers")}
        </button>

        {/* Ungrouped */}
        <button
          onClick={() => onSelect("ungrouped")}
          className={cn(
            "w-full flex items-center gap-2 px-3 py-1.5 text-sm rounded-md transition-colors",
            selectedId === "ungrouped"
              ? "bg-primary/10 text-primary font-medium"
              : "text-muted-foreground hover:bg-muted"
          )}
        >
          <InboxIcon className="size-4 shrink-0" />
          {t("departments.ungrouped")}
        </button>

        {/* Department tree */}
        {departments.length > 0 && (
          <div className="pt-2">
            <p className="px-3 pb-1 text-xs font-medium uppercase tracking-[0.1em] text-muted-foreground">
              {t("departments.title")}
            </p>
            {departments.map((dept) => (
              <DepartmentNode
                key={dept.id}
                dept={dept}
                selectedId={selectedId}
                onSelect={onSelect}
                depth={0}
              />
            ))}
          </div>
        )}
      </div>

      <div className="border-t px-3 py-2">
        <button
          onClick={onManage}
          className="w-full text-xs text-muted-foreground hover:text-foreground transition-colors text-center py-1"
        >
          {t("departments.manage")}
        </button>
      </div>
    </div>
  )
}

function DepartmentNode({
  dept,
  selectedId,
  onSelect,
  depth,
}: {
  dept: DepartmentTreeType
  selectedId: string | null
  onSelect: (id: string) => void
  depth: number
}) {
  const [expanded, setExpanded] = useState(true)
  const hasChildren = dept.children.length > 0

  return (
    <div>
      <button
        onClick={() => onSelect(dept.id)}
        className={cn(
          "w-full flex items-center gap-1.5 py-1.5 text-sm rounded-md transition-colors",
          selectedId === dept.id
            ? "bg-primary/10 text-primary font-medium"
            : "text-muted-foreground hover:bg-muted"
        )}
        style={{ paddingLeft: `${depth * 16 + 12}px`, paddingRight: "12px" }}
      >
        {hasChildren ? (
          <button
            onClick={(e) => {
              e.stopPropagation()
              setExpanded(!expanded)
            }}
            className="shrink-0 p-0.5 hover:bg-muted rounded"
          >
            <ChevronRightIcon
              className={cn("size-3.5 transition-transform", expanded && "rotate-90")}
            />
          </button>
        ) : (
          <span className="w-4.5 shrink-0" />
        )}
        {expanded && hasChildren ? (
          <FolderOpenIcon className="size-4 shrink-0" />
        ) : (
          <FolderIcon className="size-4 shrink-0" />
        )}
        <span className="truncate">{dept.name}</span>
      </button>

      {expanded && hasChildren && (
        <div>
          {dept.children.map((child) => (
            <DepartmentNode
              key={child.id}
              dept={child}
              selectedId={selectedId}
              onSelect={onSelect}
              depth={depth + 1}
            />
          ))}
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/department-tree.tsx
git commit -m "feat(web): add DepartmentTreeSidebar component"
```

---

### Task 11: Department Management Dialog

**Files:**
- Create: `web/src/components/department-dialog.tsx`

- [ ] **Step 1: Create the department management dialog**

Create `web/src/components/department-dialog.tsx`:

```tsx
import { useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { PlusIcon, PencilIcon, Trash2Icon, FolderIcon } from "lucide-react"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { useDepartments, useCreateDepartment, useUpdateDepartment, useDeleteDepartment } from "@/hooks/use-departments"
import type { Department, DepartmentTree } from "@/lib/types"

interface DepartmentManageDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function DepartmentManageDialog({ open, onOpenChange }: DepartmentManageDialogProps) {
  const { t } = useTranslation()
  const { data: departments = [] } = useDepartments()
  const createDept = useCreateDepartment()
  const updateDept = useUpdateDepartment()
  const deleteDept = useDeleteDepartment()

  const [mode, setMode] = useState<"list" | "create" | "edit">("list")
  const [editingDept, setEditingDept] = useState<Department | null>(null)
  const [name, setName] = useState("")
  const [parentId, setParentId] = useState<string | null>(null)
  const [error, setError] = useState("")

  const flatDepts = flattenTree(departments)

  const resetForm = () => {
    setName("")
    setParentId(null)
    setError("")
    setEditingDept(null)
    setMode("list")
  }

  const handleCreate = async (e?: FormEvent) => {
    e?.preventDefault()
    if (!name.trim()) return
    try {
      await createDept.mutateAsync({ name: name.trim(), parent_id: parentId })
      resetForm()
    } catch (err: any) {
      setError(err.message)
    }
  }

  const handleUpdate = async (e?: FormEvent) => {
    e?.preventDefault()
    if (!editingDept || !name.trim()) return
    try {
      await updateDept.mutateAsync({
        id: editingDept.id,
        data: { name: name.trim(), parent_id: parentId },
      })
      resetForm()
    } catch (err: any) {
      setError(err.message)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await deleteDept.mutateAsync(id)
    } catch (err: any) {
      setError(err.message)
    }
  }

  const startEdit = (dept: Department) => {
    setEditingDept(dept)
    setName(dept.name)
    setParentId(dept.parent_id)
    setError("")
    setMode("edit")
  }

  const startCreate = (parentIdVal?: string | null) => {
    setName("")
    setParentId(parentIdVal ?? null)
    setError("")
    setMode("create")
  }

  return (
    <Dialog open={open} onOpenChange={(o) => { if (!o) resetForm(); onOpenChange(o) }}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t("departments.manageTitle")}</DialogTitle>
          <DialogDescription>{t("departments.manageDescription")}</DialogDescription>
        </DialogHeader>

        {error && <p className="text-sm text-destructive">{error}</p>}

        {mode === "list" && (
          <div className="space-y-2">
            {flatDepts.length === 0 ? (
              <p className="text-sm text-muted-foreground text-center py-4">
                {t("departments.empty")}
              </p>
            ) : (
              <div className="max-h-64 overflow-y-auto space-y-0.5">
                {flatDepts.map(({ dept, depth }) => (
                  <div
                    key={dept.id}
                    className="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-muted group"
                    style={{ paddingLeft: `${depth * 16 + 8}px` }}
                  >
                    <FolderIcon className="size-4 shrink-0 text-muted-foreground" />
                    <span className="text-sm flex-1 truncate">{dept.name}</span>
                    <div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => startCreate(dept.id)}
                        title={t("departments.addChild")}
                      >
                        <PlusIcon className="size-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => startEdit(dept)}
                        title={t("departments.rename")}
                      >
                        <PencilIcon className="size-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => handleDelete(dept.id)}
                        title={t("common.delete")}
                      >
                        <Trash2Icon className="size-3.5" />
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}

            <DialogFooter className="pt-2">
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                {t("common.close")}
              </Button>
              <Button onClick={() => startCreate()}>
                <PlusIcon className="size-4 mr-1" />
                {t("departments.create")}
              </Button>
            </DialogFooter>
          </div>
        )}

        {(mode === "create" || mode === "edit") && (
          <form onSubmit={mode === "create" ? handleCreate : handleUpdate} className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="dept-name">{t("departments.form.name")}</Label>
              <Input
                id="dept-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={t("departments.form.namePlaceholder")}
                required
                autoFocus
              />
            </div>

            <div className="space-y-1.5">
              <Label>{t("departments.form.parent")}</Label>
              <Select
                value={parentId ?? "__none__"}
                onValueChange={(v) => setParentId(v === "__none__" ? null : v)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__none__">{t("departments.form.noParent")}</SelectItem>
                  {flatDepts
                    .filter(({ dept }) => dept.id !== editingDept?.id)
                    .map(({ dept, depth }) => (
                      <SelectItem key={dept.id} value={dept.id}>
                        {"  ".repeat(depth)}{dept.name}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={resetForm}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={!name.trim()}>
                {mode === "create" ? t("departments.create") : t("common.save")}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}

function flattenTree(
  tree: DepartmentTree[],
  depth = 0
): { dept: Department; depth: number }[] {
  const result: { dept: Department; depth: number }[] = []
  for (const node of tree) {
    result.push({ dept: node, depth })
    if (node.children.length > 0) {
      result.push(...flattenTree(node.children, depth + 1))
    }
  }
  return result
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/department-dialog.tsx
git commit -m "feat(web): add DepartmentManageDialog component for CRUD operations"
```

---

### Task 12: Refactor Workers Page with Department Tree Layout

**Files:**
- Modify: `web/src/pages/workers.tsx`

- [ ] **Step 1: Update Workers page to include department sidebar**

Modify `web/src/pages/workers.tsx` to add the left-right layout. The key changes are:

1. Import new components and hooks
2. Add state for `selectedDepartmentId` and `manageDeptOpen`
3. Pass `departmentId` filter to `useWorkers`
4. Wrap the existing content in a flex layout with the department tree sidebar on the left

Add imports at the top:

```typescript
import { useDepartments } from "@/hooks/use-departments"
import { DepartmentTreeSidebar } from "@/components/department-tree"
import { DepartmentManageDialog } from "@/components/department-dialog"
```

Add state inside the `Workers` component:

```typescript
const [selectedDeptId, setSelectedDeptId] = useState<string | null>(null)
const [manageDeptOpen, setManageDeptOpen] = useState(false)
const { data: departments = [] } = useDepartments()
```

Update the `useWorkers` call to pass the department filter:

```typescript
const deptFilter = selectedDeptId === "ungrouped" ? undefined : (selectedDeptId ?? undefined)
const { data: workers = [], error: fetchError, isLoading } = useWorkers(deptFilter)
```

Note: For "ungrouped", we load all workers and filter client-side to show only those with empty `departments` array. Update the workers display logic:

```typescript
const displayedWorkers = selectedDeptId === "ungrouped"
  ? workers.filter((w) => !w.departments || w.departments.length === 0)
  : workers
```

Wrap the entire return JSX in a flex layout:

```tsx
return (
  <FadeIn>
    <div className="flex gap-6 h-full">
      {/* Left sidebar */}
      <div className="w-56 shrink-0 border-r pr-4">
        <DepartmentTreeSidebar
          departments={departments}
          selectedId={selectedDeptId}
          onSelect={setSelectedDeptId}
          onManage={() => setManageDeptOpen(true)}
        />
      </div>

      {/* Right content */}
      <div className="flex-1 min-w-0">
        {/* ... existing PageHeader, table, dialogs ... */}
        {/* Use displayedWorkers instead of workers in the table rendering */}
      </div>
    </div>

    <DepartmentManageDialog open={manageDeptOpen} onOpenChange={setManageDeptOpen} />
  </FadeIn>
)
```

Replace `workers.map(...)` in the table body with `displayedWorkers.map(...)`.

Update the `activeWorkers` count and `workers.length` references to use `displayedWorkers` where appropriate for the currently displayed view.

- [ ] **Step 2: Verify the frontend compiles**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee/web && npm run build`
Expected: build succeeds (there may be missing i18n keys — those are warnings, not errors)

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/workers.tsx
git commit -m "feat(web): add department tree sidebar to Workers page with filtering"
```

---

### Task 13: Worker Detail — Department Selection

**Files:**
- Modify: `web/src/pages/worker-detail.tsx`

- [ ] **Step 1: Add department selection to worker detail page**

In the worker detail page, add a section showing the worker's departments with an edit button. This section should:

1. Display current departments as badges/tags
2. On click, open a multi-select dialog for choosing departments
3. Use `useSetWorkerDepartments` mutation to save changes

Add imports:

```typescript
import { useDepartments, useSetWorkerDepartments } from "@/hooks/use-departments"
```

Add the hook calls and state inside the component:

```typescript
const { data: departments = [] } = useDepartments()
const setWorkerDepts = useSetWorkerDepartments()
const [deptDialogOpen, setDeptDialogOpen] = useState(false)
```

Add a departments display section in the worker detail view (after the existing description/memory fields). Show the worker's current departments from the `worker.departments` field as clickable badges that open the selection dialog.

The department selection dialog should show all departments in a flat checklist (with indentation for hierarchy), with checkboxes. On save, call `setWorkerDepts.mutateAsync({ workerId: worker.id, departmentIds: selectedIds })`.

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee/web && npm run build`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/worker-detail.tsx
git commit -m "feat(web): add department selection to worker detail page"
```

---

### Task 14: Add i18n Translation Keys

**Files:**
- Modify: i18n translation files (find them via `web/src/i18n/` or `web/src/locales/`)

- [ ] **Step 1: Find and update translation files**

Search for existing i18n files:

Run: `find web/src -name "*.json" -path "*/locales/*" -o -name "*.json" -path "*/i18n/*" | head -20`

Add the following keys to both English and Chinese translation files:

English:
```json
{
  "departments": {
    "title": "Departments",
    "allWorkers": "All Workers",
    "ungrouped": "Ungrouped",
    "manage": "Manage Departments",
    "manageTitle": "Manage Departments",
    "manageDescription": "Create, edit, and organize departments for your workers.",
    "empty": "No departments yet. Create one to get started.",
    "create": "Create Department",
    "addChild": "Add Sub-department",
    "rename": "Rename",
    "form": {
      "name": "Department Name",
      "namePlaceholder": "e.g. Engineering",
      "parent": "Parent Department",
      "noParent": "None (Top Level)"
    }
  }
}
```

Chinese:
```json
{
  "departments": {
    "title": "部门",
    "allWorkers": "全部 Worker",
    "ungrouped": "未分组",
    "manage": "管理部门",
    "manageTitle": "管理部门",
    "manageDescription": "创建、编辑和组织 Worker 的部门结构。",
    "empty": "暂无部门，创建一个开始使用。",
    "create": "创建部门",
    "addChild": "添加子部门",
    "rename": "重命名",
    "form": {
      "name": "部门名称",
      "namePlaceholder": "例如：技术部",
      "parent": "上级部门",
      "noParent": "无（顶级部门）"
    }
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/
git commit -m "feat(i18n): add department-related translation keys for en and zh"
```

---

### Task 15: Final Verification

**Files:** None (verification only)

- [ ] **Step 1: Run all Go tests**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./...`
Expected: all tests pass

- [ ] **Step 2: Build Go binary**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./...`
Expected: no errors

- [ ] **Step 3: Build frontend**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee/web && npm run build`
Expected: no errors

- [ ] **Step 4: Verify all new API endpoints exist in router**

Run: `grep -n "departments\|/departments" internal/api/router.go`
Expected: see all department routes registered

- [ ] **Step 5: Verify migrations are in correct order**

Run: `grep "version:" internal/infra/store/db.go | tail -10`
Expected: versions 22-26 for department tables and indexes
