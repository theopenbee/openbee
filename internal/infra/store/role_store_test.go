package store

import (
	"testing"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func setupRoleStore(t *testing.T) *RoleStore {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewRoleStore(db)
}

func TestRoleStore_CreateAndGet(t *testing.T) {
	rs := setupRoleStore(t)
	created, err := rs.Create(model.Role{Name: "ops"}, []string{"contacts:read", "tasks:read"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty id")
	}
	got, err := rs.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "ops" || len(got.Permissions) != 2 {
		t.Fatalf("unexpected role: %+v", got)
	}
}

func TestRoleStore_SeedRolesPresent(t *testing.T) {
	rs := setupRoleStore(t)
	roles, err := rs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(roles) != 1 {
		t.Fatalf("expected 1 seed role, got %d", len(roles))
	}
	if roles[0].ID != model.RoleIDSuperAdmin {
		t.Fatalf("expected super-admin seed role, got %s", roles[0].ID)
	}
}

func TestRoleStore_UpdatePermissions(t *testing.T) {
	rs := setupRoleStore(t)
	r, _ := rs.Create(model.Role{Name: "ops"}, []string{"contacts:read"})
	r.Description = "operations"
	if err := rs.Update(r.Role, []string{"contacts:read", "contacts:write"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := rs.GetByID(r.ID)
	if got.Description != "operations" || len(got.Permissions) != 2 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestRoleStore_DeleteSystemRoleBlocked(t *testing.T) {
	rs := setupRoleStore(t)
	err := rs.Delete(model.RoleIDSuperAdmin)
	if err == nil {
		t.Fatal("expected deleting a system role to fail")
	}
}

func TestRoleStore_DeleteCustomRole(t *testing.T) {
	rs := setupRoleStore(t)
	r, _ := rs.Create(model.Role{Name: "ops"}, []string{"contacts:read"})
	if err := rs.Delete(r.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := rs.GetByID(r.ID); err == nil {
		t.Fatal("expected role to be gone")
	}
}
