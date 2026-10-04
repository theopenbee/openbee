package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func setupDeptTestDB(t *testing.T) (*DepartmentStore, *WorkerStore) {
	t.Helper()
	db := newTestDB(t)
	return NewDepartmentStore(db), NewWorkerStore(db)
}

func TestDepartmentStore_Create(t *testing.T) {
	ds, _ := setupDeptTestDB(t)
	d, err := ds.Create(model.Department{Name: "Engineering"})
	require.NoError(t, err)
	assert.NotEmpty(t, d.ID)
	assert.Equal(t, "Engineering", d.Name)
}

func TestDepartmentStore_GetByID(t *testing.T) {
	ds, _ := setupDeptTestDB(t)
	d, _ := ds.Create(model.Department{Name: "Sales"})
	got, err := ds.GetByID(d.ID)
	require.NoError(t, err)
	assert.Equal(t, "Sales", got.Name)
}

func TestDepartmentStore_Update(t *testing.T) {
	ds, _ := setupDeptTestDB(t)
	d, _ := ds.Create(model.Department{Name: "Old"})
	d.Name = "New"
	updated, err := ds.Update(d)
	require.NoError(t, err)
	assert.Equal(t, "New", updated.Name)
}

func TestDepartmentStore_Delete_Empty(t *testing.T) {
	ds, _ := setupDeptTestDB(t)
	d, _ := ds.Create(model.Department{Name: "ToDelete"})
	require.NoError(t, ds.Delete(d.ID))
	_, err := ds.GetByID(d.ID)
	assert.Error(t, err)
}

func TestDepartmentStore_Delete_HasChildren(t *testing.T) {
	ds, _ := setupDeptTestDB(t)
	parent, _ := ds.Create(model.Department{Name: "Parent"})
	ds.Create(model.Department{Name: "Child", ParentID: &parent.ID})
	err := ds.Delete(parent.ID)
	assert.Error(t, err)
}

func TestDepartmentStore_Delete_HasWorkers(t *testing.T) {
	ds, ws := setupDeptTestDB(t)
	dept, _ := ds.Create(model.Department{Name: "Dept"})
	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	ds.SetWorkerDepartments(w.ID, []string{dept.ID})
	err := ds.Delete(dept.ID)
	assert.Error(t, err)
}

func TestDepartmentStore_BuildTree(t *testing.T) {
	ds, _ := setupDeptTestDB(t)
	root, _ := ds.Create(model.Department{Name: "Root", SortOrder: 0})
	child, _ := ds.Create(model.Department{Name: "Child", ParentID: &root.ID, SortOrder: 0})
	ds.Create(model.Department{Name: "Grandchild", ParentID: &child.ID, SortOrder: 0})

	all, _ := ds.ListAll()
	tree := ds.BuildTree(all)

	require.Len(t, tree, 1)
	assert.Equal(t, "Root", tree[0].Name)
	require.Len(t, tree[0].Children, 1)
	require.Len(t, tree[0].Children[0].Children, 1)
}

func TestDepartmentStore_BuildTree_SortOrder(t *testing.T) {
	ds, _ := setupDeptTestDB(t)
	ds.Create(model.Department{Name: "B", SortOrder: 2})
	ds.Create(model.Department{Name: "A", SortOrder: 1})
	ds.Create(model.Department{Name: "C", SortOrder: 3})

	all, _ := ds.ListAll()
	tree := ds.BuildTree(all)

	require.Len(t, tree, 3)
	assert.Equal(t, "A", tree[0].Name)
	assert.Equal(t, "B", tree[1].Name)
	assert.Equal(t, "C", tree[2].Name)
}

func TestDepartmentStore_SetWorkerDepartments(t *testing.T) {
	ds, ws := setupDeptTestDB(t)
	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	d1, _ := ds.Create(model.Department{Name: "Dept1"})
	d2, _ := ds.Create(model.Department{Name: "Dept2"})

	// Set initial departments
	require.NoError(t, ds.SetWorkerDepartments(w.ID, []string{d1.ID, d2.ID}))
	depts, _ := ds.GetWorkerDepartments(w.ID)
	assert.Len(t, depts, 2)

	// Replace with just one
	require.NoError(t, ds.SetWorkerDepartments(w.ID, []string{d1.ID}))
	depts, _ = ds.GetWorkerDepartments(w.ID)
	assert.Len(t, depts, 1)

	// Clear all
	require.NoError(t, ds.SetWorkerDepartments(w.ID, []string{}))
	depts, _ = ds.GetWorkerDepartments(w.ID)
	assert.Empty(t, depts)
}

func TestDepartmentStore_GetWorkerIDsForDepartments(t *testing.T) {
	ds, ws := setupDeptTestDB(t)
	dept, _ := ds.Create(model.Department{Name: "Dept"})
	w1, _ := ws.Create(model.Worker{Name: "Bot1", WorkDir: "/tmp/b1"})
	w2, _ := ws.Create(model.Worker{Name: "Bot2", WorkDir: "/tmp/b2"})

	ds.SetWorkerDepartments(w1.ID, []string{dept.ID})
	ds.SetWorkerDepartments(w2.ID, []string{dept.ID})

	ids, err := ds.GetWorkerIDsForDepartments([]string{dept.ID})
	require.NoError(t, err)
	assert.Len(t, ids, 2)
}

func TestDepartmentStore_CheckCircularReference(t *testing.T) {
	ds, _ := setupDeptTestDB(t)
	a, _ := ds.Create(model.Department{Name: "A"})
	b, _ := ds.Create(model.Department{Name: "B", ParentID: &a.ID})
	c, _ := ds.Create(model.Department{Name: "C", ParentID: &b.ID})

	// Moving A under C would create A -> B -> C -> A cycle
	err := ds.CheckCircularReference(a.ID, c.ID)
	assert.Error(t, err)

	// Moving C under A is fine (already the case via B)
	err = ds.CheckCircularReference(c.ID, a.ID)
	assert.NoError(t, err)
}
