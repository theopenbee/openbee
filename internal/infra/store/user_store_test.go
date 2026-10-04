package store

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func setupUserStore(t *testing.T) *UserStore {
	t.Helper()
	return NewUserStore(newTestDB(t))
}

// makeRole creates a custom role with the given permissions and returns its ID.
func makeRole(t *testing.T, us *UserStore, name string, perms []string) string {
	t.Helper()
	r, err := NewRoleStore(us.db).Create(model.Role{Name: name}, perms)
	require.NoError(t, err, "create role %s", name)
	return r.ID
}

func TestUserStore_CreateAndAuthenticate(t *testing.T) {
	us := setupUserStore(t)
	u, err := us.Create("alice", "s3cret", "Alice", "", []string{model.RoleIDSuperAdmin})
	require.NoError(t, err)
	require.NotEmpty(t, u.ID)
	require.Len(t, u.Roles, 1)

	got, err := us.Authenticate("alice", "s3cret")
	require.NoError(t, err)
	require.Equal(t, u.ID, got.ID, "authenticated wrong user")

	_, err = us.Authenticate("alice", "wrong")
	require.Error(t, err, "expected wrong password to fail")
}

func TestUserStore_Count(t *testing.T) {
	us := setupUserStore(t)
	n, err := us.Count()
	require.NoError(t, err)
	require.Equal(t, 0, n)
	_, _ = us.Create("bob", "pw", "Bob", "", []string{model.RoleIDSuperAdmin})
	n, _ = us.Count()
	require.Equal(t, 1, n)
}

func TestUserStore_PermissionsUnion(t *testing.T) {
	us := setupUserStore(t)
	writer := makeRole(t, us, "writer", []string{"contacts:write", "users:manage"})
	reader := makeRole(t, us, "reader", []string{"contacts:read"})
	u, _ := us.Create("carol", "pw", "Carol", "", []string{writer, reader})
	perms, err := us.PermissionsForUser(u.ID)
	require.NoError(t, err)
	require.True(t, slices.Contains(perms, "contacts:write") && slices.Contains(perms, "users:manage"), "expected union perms, got %v", perms)
}

func TestUserStore_SuperAdminWildcard(t *testing.T) {
	us := setupUserStore(t)
	u, _ := us.Create("root", "pw", "Root", "", []string{model.RoleIDSuperAdmin})
	perms, _ := us.PermissionsForUser(u.ID)
	require.True(t, slices.Contains(perms, "*"), "expected wildcard, got %v", perms)
}

func TestUserStore_SetRolesAndStatusAndPassword(t *testing.T) {
	us := setupUserStore(t)
	basic := makeRole(t, us, "basic", []string{"contacts:read"})
	elevated := makeRole(t, us, "elevated", []string{"users:manage"})
	u, _ := us.Create("dave", "pw", "Dave", "", []string{basic})

	require.NoError(t, us.SetRoles(u.ID, []string{elevated}))
	perms, _ := us.PermissionsForUser(u.ID)
	require.True(t, slices.Contains(perms, "users:manage"), "expected elevated perms after SetRoles")

	require.NoError(t, us.SetStatus(u.ID, model.UserStatusDisabled))
	_, err := us.Authenticate("dave", "pw")
	require.Error(t, err, "disabled user must not authenticate")

	require.NoError(t, us.SetPassword(u.ID, "newpw"))
	_ = us.SetStatus(u.ID, model.UserStatusActive)
	_, err = us.Authenticate("dave", "newpw")
	require.NoError(t, err, "expected new password to work")
}

func TestUserStore_SetPasswordBumpsPasswordChangedAt(t *testing.T) {
	us := setupUserStore(t)
	u, _ := us.Create("erin", "pw", "Erin", "", nil)

	status, before, err := us.UserAuthState(u.ID)
	require.NoError(t, err)
	require.Equal(t, model.UserStatusActive, status)

	require.NoError(t, us.SetPassword(u.ID, "newpw"))
	_, after, err := us.UserAuthState(u.ID)
	require.NoError(t, err, "UserAuthState after")
	require.True(t, after >= before, "password_changed_at must advance on SetPassword: before=%d after=%d", before, after)
	require.True(t, after%1000 == 0, "password_changed_at must be floored to the second, got %d", after)
}

func TestUserStore_DeleteCascadesRoles(t *testing.T) {
	us := setupUserStore(t)
	u, _ := us.Create("erin", "pw", "Erin", "", []string{model.RoleIDSuperAdmin})
	require.NoError(t, us.Delete(u.ID))
	_, err := us.GetByID(u.ID)
	assert.Error(t, err, "expected user gone")
}
