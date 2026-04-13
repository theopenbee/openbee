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

func TestDepartmentStore_GetWorkerIDsForDepartments(t *testing.T) {
	ds, ws := setupDeptTestDB(t)
	dept, _ := ds.Create(model.Department{Name: "Dept"})
	w1, _ := ws.Create(model.Worker{Name: "Bot1", WorkDir: "/tmp/b1"})
	w2, _ := ws.Create(model.Worker{Name: "Bot2", WorkDir: "/tmp/b2"})

	ds.SetWorkerDepartments(w1.ID, []string{dept.ID})
	ds.SetWorkerDepartments(w2.ID, []string{dept.ID})

	ids, err := ds.GetWorkerIDsForDepartments([]string{dept.ID})
	if err != nil {
		t.Fatalf("GetWorkerIDsForDepartments: %v", err)
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
