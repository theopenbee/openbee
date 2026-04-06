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
