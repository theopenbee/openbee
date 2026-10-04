package store

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func setupRoleStore(t *testing.T) *RoleStore {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return NewRoleStore(db)
}

func TestRoleStore_CreateAndGet(t *testing.T) {
	rs := setupRoleStore(t)
	created, err := rs.Create(model.Role{Name: "ops"}, []string{"contacts:read", "tasks:read"})
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)
	got, err := rs.GetByID(created.ID)
	require.NoError(t, err)
	require.Equal(t, "ops", got.Name)
	require.Len(t, got.Permissions, 2)
}

func TestRoleStore_SeedRolesPresent(t *testing.T) {
	rs := setupRoleStore(t)
	roles, err := rs.List()
	require.NoError(t, err)
	require.Len(t, roles, 1)
	require.Equal(t, model.RoleIDSuperAdmin, roles[0].ID)
}

func TestRoleStore_UpdatePermissions(t *testing.T) {
	rs := setupRoleStore(t)
	r, _ := rs.Create(model.Role{Name: "ops"}, []string{"contacts:read"})
	r.Description = "operations"
	require.NoError(t, rs.Update(r.Role, []string{"contacts:read", "contacts:write"}))
	got, _ := rs.GetByID(r.ID)
	require.Equal(t, "operations", got.Description)
	require.Len(t, got.Permissions, 2)
}

func TestRoleStore_DeleteSystemRoleBlocked(t *testing.T) {
	rs := setupRoleStore(t)
	err := rs.Delete(model.RoleIDSuperAdmin)
	require.Error(t, err, "expected deleting a system role to fail")
}

func TestRoleStore_DeleteCustomRole(t *testing.T) {
	rs := setupRoleStore(t)
	r, _ := rs.Create(model.Role{Name: "ops"}, []string{"contacts:read"})
	require.NoError(t, rs.Delete(r.ID))
	_, err := rs.GetByID(r.ID)
	require.Error(t, err, "expected role to be gone")
}
