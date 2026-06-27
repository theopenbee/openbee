package store

import (
	"slices"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func setupUserStore(t *testing.T) *UserStore {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewUserStore(db)
}

func TestUserStore_CreateAndAuthenticate(t *testing.T) {
	us := setupUserStore(t)
	u, err := us.Create("alice", "s3cret", "Alice", "", []string{model.RoleIDAdmin})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID == "" || len(u.Roles) != 1 {
		t.Fatalf("unexpected user: %+v", u)
	}

	got, err := us.Authenticate("alice", "s3cret")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got.ID != u.ID {
		t.Fatal("authenticated wrong user")
	}
	if _, err := us.Authenticate("alice", "wrong"); err == nil {
		t.Fatal("expected wrong password to fail")
	}
}

func TestUserStore_Count(t *testing.T) {
	us := setupUserStore(t)
	n, err := us.Count()
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 users, got %d", n)
	}
	_, _ = us.Create("bob", "pw", "Bob", "", []string{model.RoleIDMember})
	n, _ = us.Count()
	if n != 1 {
		t.Fatalf("expected 1 user, got %d", n)
	}
}

func TestUserStore_PermissionsUnion(t *testing.T) {
	us := setupUserStore(t)
	u, _ := us.Create("carol", "pw", "Carol", "", []string{model.RoleIDAdmin, model.RoleIDMember})
	perms, err := us.PermissionsForUser(u.ID)
	if err != nil {
		t.Fatalf("PermissionsForUser: %v", err)
	}
	if !slices.Contains(perms, "workers:write") || !slices.Contains(perms, "users:manage") {
		t.Fatalf("expected admin perms in union, got %v", perms)
	}
}

func TestUserStore_SuperAdminWildcard(t *testing.T) {
	us := setupUserStore(t)
	u, _ := us.Create("root", "pw", "Root", "", []string{model.RoleIDSuperAdmin})
	perms, _ := us.PermissionsForUser(u.ID)
	if !slices.Contains(perms, "*") {
		t.Fatalf("expected wildcard, got %v", perms)
	}
}

func TestUserStore_SetRolesAndStatusAndPassword(t *testing.T) {
	us := setupUserStore(t)
	u, _ := us.Create("dave", "pw", "Dave", "", []string{model.RoleIDMember})

	if err := us.SetRoles(u.ID, []string{model.RoleIDAdmin}); err != nil {
		t.Fatalf("SetRoles: %v", err)
	}
	perms, _ := us.PermissionsForUser(u.ID)
	if !slices.Contains(perms, "users:manage") {
		t.Fatal("expected admin perms after SetRoles")
	}

	if err := us.SetStatus(u.ID, model.UserStatusDisabled); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if _, err := us.Authenticate("dave", "pw"); err == nil {
		t.Fatal("disabled user must not authenticate")
	}

	if err := us.SetPassword(u.ID, "newpw"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	_ = us.SetStatus(u.ID, model.UserStatusActive)
	if _, err := us.Authenticate("dave", "newpw"); err != nil {
		t.Fatalf("expected new password to work: %v", err)
	}
}

func TestUserStore_DeleteCascadesRoles(t *testing.T) {
	us := setupUserStore(t)
	u, _ := us.Create("erin", "pw", "Erin", "", []string{model.RoleIDMember})
	if err := us.Delete(u.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := us.GetByID(u.ID); err == nil {
		t.Fatal("expected user gone")
	}
}
