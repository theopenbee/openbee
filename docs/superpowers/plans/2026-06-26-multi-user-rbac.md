# Multi-User + RBAC Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the single config-based admin credential with a DB-backed multi-user system where users hold one-or-more configurable roles (RBAC), and bootstrap the first super-admin through a web setup wizard.

**Architecture:** New SQLite tables (`bee_users`, `bee_roles`, `bee_role_permissions`, `bee_user_roles`) via migration v47. A permission catalog defined in Go is the single source of truth. JWT access tokens carry a `uid`; permissions are resolved server-side per request (user → roles → union of permission keys) with an in-process cache. Gin middleware loads the user and a `RequirePermission` guard gates each route. A setup wizard creates the first super-admin when the users table is empty. The React frontend gates UI by permission and routes to the wizard when uninitialized.

**Tech Stack:** Go 1.25, Gin, modernc.org/sqlite, golang-jwt/v5, golang.org/x/crypto/bcrypt, google/uuid; React + Vite + TypeScript (Tailwind, radius ≤ `sm`).

**Spec:** `docs/superpowers/specs/2026-06-26-multi-user-rbac-design.md`

---

## File Structure

**Backend — new files:**
- `internal/infra/model/user.go` — `User`, `Role`, `RoleWithPermissions`, `UserWithRoles` structs
- `internal/infra/store/role_store.go` + `role_store_test.go` — role CRUD + permissions
- `internal/infra/store/user_store.go` + `user_store_test.go` — user CRUD, bcrypt, role bindings, permission union, count
- `internal/infra/auth/permissions.go` + `permissions_test.go` — permission catalog + `PermissionResolver` (cache)
- `internal/infra/auth/password.go` + `password_test.go` — bcrypt hash/check
- `internal/infra/auth/context.go` — gin context helpers for uid
- `internal/api/setup_handler.go` + `setup_handler_test.go` — setup status + create first super-admin
- `internal/api/user_handler.go` + `user_handler_test.go` — user management endpoints
- `internal/api/role_handler.go` + `role_handler_test.go` — role + permission-catalog endpoints

**Backend — modified files:**
- `internal/infra/store/db.go` — add migration v47
- `internal/infra/auth/jwt.go` — embed/parse `uid`
- `internal/infra/auth/middleware.go` — load user + `RequirePermission`
- `internal/infra/auth/handler.go` — login against `UserStore`, add `Me` + `ChangePassword`
- `internal/routes/server.go` — `ServerParams` new fields
- `internal/routes/api.go` — register setup/user/role routes + per-route `RequirePermission`
- `internal/app/app.go` — construct new stores/handlers/resolver, rewire middleware
- `internal/infra/config/config.go` — mark `auth.username/password` deprecated (comment only)
- `CHANGELOG.md` — English entry

**Frontend — new files:**
- `web/src/pages/setup.tsx` — first-run wizard
- `web/src/pages/users.tsx` — user management
- `web/src/pages/roles.tsx` — role management
- `web/src/lib/permissions.ts` — permission keys + `hasPermission` helper

**Frontend — modified files:**
- `web/src/lib/auth.ts` — fetch `/api/me`, store current user + permissions
- `web/src/components/auth-guard.tsx` — setup-status gate
- `web/src/app.tsx` — routes for `/setup`, `/users`, `/roles`
- navigation component — hide entries by permission

---

## Phase A — Data layer

### Task 1: Migration v47 — tables + seed roles

**Files:**
- Modify: `internal/infra/store/db.go` (append to `migrations` slice after version 46)
- Test: `internal/infra/store/db_migration_v47_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/infra/store/db_migration_v47_test.go`:

```go
package store

import "testing"

func TestMigrationV47_TablesAndSeedRoles(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	for _, table := range []string{"bee_users", "bee_roles", "bee_role_permissions", "bee_user_roles"} {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Fatalf("expected table %s to exist: %v", table, err)
		}
	}

	var roleCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM bee_roles`).Scan(&roleCount); err != nil {
		t.Fatalf("count roles: %v", err)
	}
	if roleCount != 3 {
		t.Fatalf("expected 3 seed roles, got %d", roleCount)
	}

	var wildcard int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM bee_role_permissions WHERE role_id='sysrole_superadmin' AND permission='*'`,
	).Scan(&wildcard); err != nil {
		t.Fatalf("query superadmin permission: %v", err)
	}
	if wildcard != 1 {
		t.Fatalf("expected super-admin to have '*' permission")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/store/ -run TestMigrationV47 -v`
Expected: FAIL — `bee_users` table does not exist.

- [ ] **Step 3: Add the migration**

In `internal/infra/store/db.go`, append these entries to the `migrations` slice (immediately before the closing `}` of the slice, after version 46):

```go
	{
		version: 47,
		name:    "create_rbac_tables_and_seed_roles",
		sql: `
CREATE TABLE IF NOT EXISTS bee_users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name  TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','disabled')),
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS bee_roles (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    is_system   INTEGER NOT NULL DEFAULT 0,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS bee_role_permissions (
    role_id    TEXT NOT NULL REFERENCES bee_roles(id) ON DELETE CASCADE,
    permission TEXT NOT NULL,
    PRIMARY KEY (role_id, permission)
);
CREATE TABLE IF NOT EXISTS bee_user_roles (
    user_id    TEXT NOT NULL REFERENCES bee_users(id) ON DELETE CASCADE,
    role_id    TEXT NOT NULL REFERENCES bee_roles(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, role_id)
);
INSERT OR IGNORE INTO bee_roles (id, name, description, is_system, created_at, updated_at) VALUES
    ('sysrole_superadmin', 'super-admin', 'Full access; cannot be deleted or downgraded', 1, CAST(strftime('%s','now') AS INTEGER)*1000, CAST(strftime('%s','now') AS INTEGER)*1000),
    ('sysrole_admin', 'admin', 'Manage business resources and users', 0, CAST(strftime('%s','now') AS INTEGER)*1000, CAST(strftime('%s','now') AS INTEGER)*1000),
    ('sysrole_member', 'member', 'Read-only access to business resources', 0, CAST(strftime('%s','now') AS INTEGER)*1000, CAST(strftime('%s','now') AS INTEGER)*1000);
INSERT OR IGNORE INTO bee_role_permissions (role_id, permission) VALUES
    ('sysrole_superadmin', '*'),
    ('sysrole_admin', 'workers:read'), ('sysrole_admin', 'workers:write'),
    ('sysrole_admin', 'tasks:read'), ('sysrole_admin', 'tasks:write'),
    ('sysrole_admin', 'departments:read'), ('sysrole_admin', 'departments:write'),
    ('sysrole_admin', 'messages:read'),
    ('sysrole_admin', 'sessions:read'), ('sysrole_admin', 'sessions:write'),
    ('sysrole_admin', 'stats:read'),
    ('sysrole_admin', 'env:read'), ('sysrole_admin', 'env:write'),
    ('sysrole_admin', 'system_config:read'),
    ('sysrole_admin', 'users:manage'),
    ('sysrole_member', 'workers:read'), ('sysrole_member', 'tasks:read'),
    ('sysrole_member', 'departments:read'), ('sysrole_member', 'messages:read'),
    ('sysrole_member', 'sessions:read'), ('sysrole_member', 'stats:read'),
    ('sysrole_member', 'env:read'), ('sysrole_member', 'system_config:read');
`,
	},
```

> Note: the migration runner applies each `sql` blob; multi-statement blobs are already used (see v40/v45). `PRAGMA foreign_keys` is enabled by `InitDB`; verify that ON DELETE CASCADE works in tests (Task 4).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/infra/store/ -run TestMigrationV47 -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/db.go internal/infra/store/db_migration_v47_test.go
git commit -m "feat(store): add RBAC tables and seed roles (migration v47)"
```

---

### Task 2: User & Role models

**Files:**
- Create: `internal/infra/model/user.go`
- Test: none (pure structs; covered by store tests)

- [ ] **Step 1: Write the models**

Create `internal/infra/model/user.go`:

```go
package model

// User is a human account that can log in to the web console.
type User struct {
	ID           string `json:"id" db:"id"`
	Username     string `json:"username" db:"username"`
	PasswordHash string `json:"-" db:"password_hash"`
	DisplayName  string `json:"display_name" db:"display_name"`
	Status       string `json:"status" db:"status"`
	CreatedBy    string `json:"created_by" db:"created_by"`
	CreatedAt    int64  `json:"created_at" db:"created_at"`
	UpdatedAt    int64  `json:"updated_at" db:"updated_at"`
}

// UserWithRoles is a user plus its assigned roles, for list/detail responses.
type UserWithRoles struct {
	User
	Roles []Role `json:"roles"`
}

// Role is an RBAC role.
type Role struct {
	ID          string `json:"id" db:"id"`
	Name        string `json:"name" db:"name"`
	Description string `json:"description" db:"description"`
	IsSystem    bool   `json:"is_system" db:"is_system"`
	CreatedAt   int64  `json:"created_at" db:"created_at"`
	UpdatedAt   int64  `json:"updated_at" db:"updated_at"`
}

// RoleWithPermissions is a role plus its permission keys.
type RoleWithPermissions struct {
	Role
	Permissions []string `json:"permissions"`
}

// User status constants.
const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

// Seed system role IDs (inserted by migration v47).
const (
	RoleIDSuperAdmin = "sysrole_superadmin"
	RoleIDAdmin      = "sysrole_admin"
	RoleIDMember     = "sysrole_member"
)
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/infra/model/`
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add internal/infra/model/user.go
git commit -m "feat(model): add User and Role models"
```

---

### Task 3: RoleStore

**Files:**
- Create: `internal/infra/store/role_store.go`
- Test: `internal/infra/store/role_store_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/infra/store/role_store_test.go`:

```go
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
	created, err := rs.Create(model.Role{Name: "ops"}, []string{"workers:read", "tasks:read"})
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
	if len(roles) != 3 {
		t.Fatalf("expected 3 seed roles, got %d", len(roles))
	}
}

func TestRoleStore_UpdatePermissions(t *testing.T) {
	rs := setupRoleStore(t)
	r, _ := rs.Create(model.Role{Name: "ops"}, []string{"workers:read"})
	r.Description = "operations"
	if err := rs.Update(r, []string{"workers:read", "workers:write"}); err != nil {
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
	r, _ := rs.Create(model.Role{Name: "ops"}, []string{"workers:read"})
	if err := rs.Delete(r.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := rs.GetByID(r.ID); err == nil {
		t.Fatal("expected role to be gone")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/store/ -run TestRoleStore -v`
Expected: FAIL — `NewRoleStore` undefined.

- [ ] **Step 3: Implement RoleStore**

Create `internal/infra/store/role_store.go`:

```go
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type RoleStore struct {
	db *sql.DB
}

func NewRoleStore(db *sql.DB) *RoleStore {
	return &RoleStore{db: db}
}

func (s *RoleStore) Create(r model.Role, permissions []string) (model.RoleWithPermissions, error) {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	now := time.Now().UnixMilli()
	r.CreatedAt, r.UpdatedAt = now, now

	tx, err := s.db.Begin()
	if err != nil {
		return model.RoleWithPermissions{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(
		`INSERT INTO bee_roles (id, name, description, is_system, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.Description, boolToInt(r.IsSystem), r.CreatedAt, r.UpdatedAt,
	); err != nil {
		return model.RoleWithPermissions{}, fmt.Errorf("insert role: %w", err)
	}
	if err := insertRolePermissions(tx, r.ID, permissions); err != nil {
		return model.RoleWithPermissions{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.RoleWithPermissions{}, err
	}
	return model.RoleWithPermissions{Role: r, Permissions: permissions}, nil
}

func (s *RoleStore) GetByID(id string) (model.RoleWithPermissions, error) {
	row := s.db.QueryRow(
		`SELECT id, name, description, is_system, created_at, updated_at FROM bee_roles WHERE id = ?`, id)
	r, err := scanRole(row)
	if err != nil {
		return model.RoleWithPermissions{}, fmt.Errorf("get role: %w", err)
	}
	perms, err := s.permissionsFor(id)
	if err != nil {
		return model.RoleWithPermissions{}, err
	}
	return model.RoleWithPermissions{Role: r, Permissions: perms}, nil
}

func (s *RoleStore) List() ([]model.RoleWithPermissions, error) {
	rows, err := s.db.Query(
		`SELECT id, name, description, is_system, created_at, updated_at FROM bee_roles ORDER BY is_system DESC, name`)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()
	var out []model.RoleWithPermissions
	for rows.Next() {
		r, err := scanRole(rows)
		if err != nil {
			return nil, err
		}
		perms, err := s.permissionsFor(r.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, model.RoleWithPermissions{Role: r, Permissions: perms})
	}
	return out, rows.Err()
}

func (s *RoleStore) Update(r model.Role, permissions []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(
		`UPDATE bee_roles SET name = ?, description = ?, updated_at = ? WHERE id = ?`,
		r.Name, r.Description, time.Now().UnixMilli(), r.ID,
	); err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	if r.ID == model.RoleIDSuperAdmin {
		// super-admin permissions are immutable; ignore incoming changes.
		return tx.Commit()
	}
	if _, err := tx.Exec(`DELETE FROM bee_role_permissions WHERE role_id = ?`, r.ID); err != nil {
		return err
	}
	if err := insertRolePermissions(tx, r.ID, permissions); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *RoleStore) Delete(id string) error {
	var isSystem int
	if err := s.db.QueryRow(`SELECT is_system FROM bee_roles WHERE id = ?`, id).Scan(&isSystem); err != nil {
		return fmt.Errorf("get role: %w", err)
	}
	if isSystem == 1 {
		return errors.New("system roles cannot be deleted")
	}
	if _, err := s.db.Exec(`DELETE FROM bee_roles WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	return nil
}

func (s *RoleStore) permissionsFor(roleID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT permission FROM bee_role_permissions WHERE role_id = ? ORDER BY permission`, roleID)
	if err != nil {
		return nil, fmt.Errorf("get role permissions: %w", err)
	}
	defer rows.Close()
	perms := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

func insertRolePermissions(tx *sql.Tx, roleID string, permissions []string) error {
	for _, p := range permissions {
		if p == "" {
			continue
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO bee_role_permissions (role_id, permission) VALUES (?, ?)`, roleID, p,
		); err != nil {
			return fmt.Errorf("insert role permission %s: %w", p, err)
		}
	}
	return nil
}

func scanRole(scanner interface{ Scan(...any) error }) (model.Role, error) {
	var r model.Role
	var isSystem int
	err := scanner.Scan(&r.ID, &r.Name, &r.Description, &isSystem, &r.CreatedAt, &r.UpdatedAt)
	r.IsSystem = isSystem == 1
	return r, err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
```

> If `boolToInt` already exists elsewhere in the `store` package, delete this copy and reuse the existing one (run `grep -rn "func boolToInt" internal/infra/store` first).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/infra/store/ -run TestRoleStore -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/role_store.go internal/infra/store/role_store_test.go
git commit -m "feat(store): add RoleStore with permission CRUD"
```

---

### Task 4: UserStore

**Files:**
- Create: `internal/infra/store/user_store.go`
- Test: `internal/infra/store/user_store_test.go`
- Depends on: Task 6 (`auth.HashPassword`) — implement Task 6 first, or inline bcrypt here and refactor. **Implement Task 6 before this task.**

- [ ] **Step 1: Write the failing test**

Create `internal/infra/store/user_store_test.go`:

```go
package store

import (
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
	if !contains(perms, "workers:write") || !contains(perms, "users:manage") {
		t.Fatalf("expected admin perms in union, got %v", perms)
	}
}

func TestUserStore_SuperAdminWildcard(t *testing.T) {
	us := setupUserStore(t)
	u, _ := us.Create("root", "pw", "Root", "", []string{model.RoleIDSuperAdmin})
	perms, _ := us.PermissionsForUser(u.ID)
	if !contains(perms, "*") {
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
	if !contains(perms, "users:manage") {
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

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/store/ -run TestUserStore -v`
Expected: FAIL — `NewUserStore` undefined.

- [ ] **Step 3: Implement UserStore**

Create `internal/infra/store/user_store.go`:

```go
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
)

var ErrInvalidCredentials = errors.New("invalid username or password")

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

const userColumns = `id, username, password_hash, display_name, status, created_by, created_at, updated_at`

func (s *UserStore) Create(username, plainPassword, displayName, createdBy string, roleIDs []string) (model.UserWithRoles, error) {
	hash, err := auth.HashPassword(plainPassword)
	if err != nil {
		return model.UserWithRoles{}, err
	}
	u := model.User{
		ID:           uuid.New().String(),
		Username:     username,
		PasswordHash: hash,
		DisplayName:  displayName,
		Status:       model.UserStatusActive,
		CreatedBy:    createdBy,
	}
	now := time.Now().UnixMilli()
	u.CreatedAt, u.UpdatedAt = now, now

	tx, err := s.db.Begin()
	if err != nil {
		return model.UserWithRoles{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(
		`INSERT INTO bee_users (`+userColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.PasswordHash, u.DisplayName, u.Status, u.CreatedBy, u.CreatedAt, u.UpdatedAt,
	); err != nil {
		return model.UserWithRoles{}, fmt.Errorf("insert user: %w", err)
	}
	if err := replaceUserRoles(tx, u.ID, roleIDs, now); err != nil {
		return model.UserWithRoles{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.UserWithRoles{}, err
	}
	return s.GetByID(u.ID)
}

func (s *UserStore) GetByID(id string) (model.UserWithRoles, error) {
	row := s.db.QueryRow(`SELECT `+userColumns+` FROM bee_users WHERE id = ?`, id)
	u, err := scanUser(row)
	if err != nil {
		return model.UserWithRoles{}, fmt.Errorf("get user: %w", err)
	}
	roles, err := s.rolesFor(id)
	if err != nil {
		return model.UserWithRoles{}, err
	}
	return model.UserWithRoles{User: u, Roles: roles}, nil
}

func (s *UserStore) List() ([]model.UserWithRoles, error) {
	rows, err := s.db.Query(`SELECT ` + userColumns + ` FROM bee_users ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var out []model.UserWithRoles
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		roles, err := s.rolesFor(u.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, model.UserWithRoles{User: u, Roles: roles})
	}
	return out, rows.Err()
}

func (s *UserStore) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM bee_users`).Scan(&n)
	return n, err
}

// Authenticate verifies credentials and rejects disabled users.
func (s *UserStore) Authenticate(username, plainPassword string) (model.UserWithRoles, error) {
	row := s.db.QueryRow(`SELECT `+userColumns+` FROM bee_users WHERE username = ?`, username)
	u, err := scanUser(row)
	if err != nil {
		return model.UserWithRoles{}, ErrInvalidCredentials
	}
	if u.Status != model.UserStatusActive {
		return model.UserWithRoles{}, ErrInvalidCredentials
	}
	if !auth.CheckPassword(u.PasswordHash, plainPassword) {
		return model.UserWithRoles{}, ErrInvalidCredentials
	}
	roles, err := s.rolesFor(u.ID)
	if err != nil {
		return model.UserWithRoles{}, err
	}
	return model.UserWithRoles{User: u, Roles: roles}, nil
}

func (s *UserStore) SetStatus(id, status string) error {
	_, err := s.db.Exec(`UPDATE bee_users SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UnixMilli(), id)
	return err
}

func (s *UserStore) SetPassword(id, plainPassword string) error {
	hash, err := auth.HashPassword(plainPassword)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE bee_users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		hash, time.Now().UnixMilli(), id)
	return err
}

func (s *UserStore) SetRoles(userID string, roleIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if err := replaceUserRoles(tx, userID, roleIDs, time.Now().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *UserStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM bee_users WHERE id = ?`, id)
	return err
}

// PermissionsForUser returns the union of permission keys across the user's roles.
func (s *UserStore) PermissionsForUser(userID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT rp.permission
		FROM bee_user_roles ur
		JOIN bee_role_permissions rp ON rp.role_id = ur.role_id
		WHERE ur.user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("permissions for user: %w", err)
	}
	defer rows.Close()
	perms := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

func (s *UserStore) rolesFor(userID string) ([]model.Role, error) {
	rows, err := s.db.Query(`
		SELECT r.id, r.name, r.description, r.is_system, r.created_at, r.updated_at
		FROM bee_user_roles ur
		JOIN bee_roles r ON r.id = ur.role_id
		WHERE ur.user_id = ?
		ORDER BY r.is_system DESC, r.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("roles for user: %w", err)
	}
	defer rows.Close()
	roles := []model.Role{}
	for rows.Next() {
		r, err := scanRole(rows)
		if err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}

func replaceUserRoles(tx *sql.Tx, userID string, roleIDs []string, now int64) error {
	if _, err := tx.Exec(`DELETE FROM bee_user_roles WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for _, rid := range roleIDs {
		if rid == "" {
			continue
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO bee_user_roles (user_id, role_id, created_at) VALUES (?, ?, ?)`,
			userID, rid, now,
		); err != nil {
			return fmt.Errorf("bind role %s: %w", rid, err)
		}
	}
	return nil
}

func scanUser(scanner interface{ Scan(...any) error }) (model.User, error) {
	var u model.User
	err := scanner.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Status, &u.CreatedBy, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}
```

> `store` importing `auth` is acceptable (auth has no dependency on store). Confirm there is no import cycle after Task 6 by running `go build ./...`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/infra/store/ -run TestUserStore -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/user_store.go internal/infra/store/user_store_test.go
git commit -m "feat(store): add UserStore with bcrypt auth and role bindings"
```

---

## Phase B — Permissions & auth core

### Task 6: bcrypt password helper

**Files:**
- Create: `internal/infra/auth/password.go`
- Test: `internal/infra/auth/password_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/infra/auth/password_test.go`:

```go
package auth

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("s3cret")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "s3cret" || hash == "" {
		t.Fatal("hash must not equal plaintext or be empty")
	}
	if !CheckPassword(hash, "s3cret") {
		t.Fatal("expected correct password to match")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("expected wrong password to fail")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/auth/ -run TestHashAndCheckPassword -v`
Expected: FAIL — `HashPassword` undefined.

- [ ] **Step 3: Implement**

Create `internal/infra/auth/password.go`:

```go
package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword returns a bcrypt hash of the plaintext password.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword reports whether plain matches the bcrypt hash.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/infra/auth/ -run TestHashAndCheckPassword -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/infra/auth/password.go internal/infra/auth/password_test.go
git commit -m "feat(auth): add bcrypt password helpers"
```

---

### Task 7: Permission catalog + resolver

**Files:**
- Create: `internal/infra/auth/permissions.go`
- Test: `internal/infra/auth/permissions_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/infra/auth/permissions_test.go`:

```go
package auth

import (
	"sync/atomic"
	"testing"
)

func TestResolver_HasPermissionWildcard(t *testing.T) {
	loader := func(uid string) ([]string, error) { return []string{"*"}, nil }
	r := NewPermissionResolver(loader)
	ok, err := r.HasPermission("u1", PermWorkersWrite)
	if err != nil {
		t.Fatalf("HasPermission: %v", err)
	}
	if !ok {
		t.Fatal("wildcard should grant any permission")
	}
}

func TestResolver_HasPermissionExact(t *testing.T) {
	loader := func(uid string) ([]string, error) { return []string{PermWorkersRead}, nil }
	r := NewPermissionResolver(loader)
	if ok, _ := r.HasPermission("u1", PermWorkersRead); !ok {
		t.Fatal("expected workers:read granted")
	}
	if ok, _ := r.HasPermission("u1", PermWorkersWrite); ok {
		t.Fatal("expected workers:write denied")
	}
}

func TestResolver_CacheAndInvalidate(t *testing.T) {
	var calls int64
	loader := func(uid string) ([]string, error) {
		atomic.AddInt64(&calls, 1)
		return []string{PermWorkersRead}, nil
	}
	r := NewPermissionResolver(loader)
	_, _ = r.HasPermission("u1", PermWorkersRead)
	_, _ = r.HasPermission("u1", PermWorkersRead)
	if atomic.LoadInt64(&calls) != 1 {
		t.Fatalf("expected loader called once (cached), got %d", calls)
	}
	r.Invalidate("u1")
	_, _ = r.HasPermission("u1", PermWorkersRead)
	if atomic.LoadInt64(&calls) != 2 {
		t.Fatalf("expected loader called again after invalidate, got %d", calls)
	}
}

func TestCatalogGroupsCoverAllPermissions(t *testing.T) {
	seen := map[string]bool{}
	for _, g := range PermissionCatalog() {
		for _, p := range g.Permissions {
			seen[p] = true
		}
	}
	for _, p := range AllPermissions() {
		if !seen[p] {
			t.Fatalf("permission %s missing from catalog groups", p)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/auth/ -run 'TestResolver|TestCatalog' -v`
Expected: FAIL — `NewPermissionResolver` undefined.

- [ ] **Step 3: Implement**

Create `internal/infra/auth/permissions.go`:

```go
package auth

import "sync"

// PermWildcard grants every permission (super-admin).
const PermWildcard = "*"

// Permission keys ("resource:action"). Single source of truth.
const (
	PermWorkersRead     = "workers:read"
	PermWorkersWrite    = "workers:write"
	PermTasksRead       = "tasks:read"
	PermTasksWrite      = "tasks:write"
	PermDepartmentsRead = "departments:read"
	PermDepartmentsWrite = "departments:write"
	PermMessagesRead    = "messages:read"
	PermSessionsRead    = "sessions:read"
	PermSessionsWrite   = "sessions:write"
	PermStatsRead       = "stats:read"
	PermEnvRead         = "env:read"
	PermEnvWrite        = "env:write"
	PermSystemConfigRead  = "system_config:read"
	PermSystemConfigWrite = "system_config:write"
	PermUsersManage     = "users:manage"
	PermRolesManage     = "roles:manage"
)

// PermissionGroup groups permissions by resource for the catalog endpoint.
type PermissionGroup struct {
	Resource    string   `json:"resource"`
	Permissions []string `json:"permissions"`
}

// PermissionCatalog returns the grouped permission catalog for the UI.
func PermissionCatalog() []PermissionGroup {
	return []PermissionGroup{
		{Resource: "workers", Permissions: []string{PermWorkersRead, PermWorkersWrite}},
		{Resource: "tasks", Permissions: []string{PermTasksRead, PermTasksWrite}},
		{Resource: "departments", Permissions: []string{PermDepartmentsRead, PermDepartmentsWrite}},
		{Resource: "messages", Permissions: []string{PermMessagesRead}},
		{Resource: "sessions", Permissions: []string{PermSessionsRead, PermSessionsWrite}},
		{Resource: "stats", Permissions: []string{PermStatsRead}},
		{Resource: "env", Permissions: []string{PermEnvRead, PermEnvWrite}},
		{Resource: "system_config", Permissions: []string{PermSystemConfigRead, PermSystemConfigWrite}},
		{Resource: "administration", Permissions: []string{PermUsersManage, PermRolesManage}},
	}
}

// AllPermissions flattens the catalog into a single slice.
func AllPermissions() []string {
	var out []string
	for _, g := range PermissionCatalog() {
		out = append(out, g.Permissions...)
	}
	return out
}

// PermissionLoader loads the raw permission keys for a user (union across roles).
type PermissionLoader func(userID string) ([]string, error)

// PermissionResolver caches per-user permission sets and answers HasPermission.
type PermissionResolver struct {
	load  PermissionLoader
	mu    sync.RWMutex
	cache map[string]map[string]struct{}
}

func NewPermissionResolver(loader PermissionLoader) *PermissionResolver {
	return &PermissionResolver{load: loader, cache: map[string]map[string]struct{}{}}
}

func (r *PermissionResolver) permSet(userID string) (map[string]struct{}, error) {
	r.mu.RLock()
	if set, ok := r.cache[userID]; ok {
		r.mu.RUnlock()
		return set, nil
	}
	r.mu.RUnlock()

	perms, err := r.load(userID)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		set[p] = struct{}{}
	}
	r.mu.Lock()
	r.cache[userID] = set
	r.mu.Unlock()
	return set, nil
}

// HasPermission reports whether the user holds perm (or the wildcard).
func (r *PermissionResolver) HasPermission(userID, perm string) (bool, error) {
	set, err := r.permSet(userID)
	if err != nil {
		return false, err
	}
	if _, ok := set[PermWildcard]; ok {
		return true, nil
	}
	_, ok := set[perm]
	return ok, nil
}

// PermissionsFor returns the user's resolved permission keys (for /api/me).
func (r *PermissionResolver) PermissionsFor(userID string) ([]string, error) {
	set, err := r.permSet(userID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	return out, nil
}

// Invalidate drops the cached set for one user.
func (r *PermissionResolver) Invalidate(userID string) {
	r.mu.Lock()
	delete(r.cache, userID)
	r.mu.Unlock()
}

// InvalidateAll clears the entire cache (use when role permissions change).
func (r *PermissionResolver) InvalidateAll() {
	r.mu.Lock()
	r.cache = map[string]map[string]struct{}{}
	r.mu.Unlock()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/infra/auth/ -run 'TestResolver|TestCatalog' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/infra/auth/permissions.go internal/infra/auth/permissions_test.go
git commit -m "feat(auth): add permission catalog and cached resolver"
```

---

### Task 8: JWT carries uid

**Files:**
- Modify: `internal/infra/auth/jwt.go`
- Test: `internal/infra/auth/jwt_uid_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/infra/auth/jwt_uid_test.go`:

```go
package auth

import (
	"testing"
	"time"
)

func TestJWT_UserTokenRoundTrip(t *testing.T) {
	svc := NewJWTService("test-secret", time.Hour, 24*time.Hour)
	pair, err := svc.GenerateUserTokenPair("user-123")
	if err != nil {
		t.Fatalf("GenerateUserTokenPair: %v", err)
	}
	uid, err := svc.ParseAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	if uid != "user-123" {
		t.Fatalf("expected uid user-123, got %s", uid)
	}

	ruid, err := svc.ParseRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ParseRefreshToken: %v", err)
	}
	if ruid != "user-123" {
		t.Fatalf("expected refresh uid user-123, got %s", ruid)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/auth/ -run TestJWT_UserTokenRoundTrip -v`
Expected: FAIL — `GenerateUserTokenPair` undefined.

- [ ] **Step 3: Implement**

In `internal/infra/auth/jwt.go`, add a `UID` field to `Claims` and add the new methods. Replace the `Claims` struct definition and append the methods:

```go
type Claims struct {
	Type string `json:"type"`
	UID  string `json:"uid,omitempty"`
	jwt.RegisteredClaims
}
```

Add these methods at the end of the file:

```go
// GenerateUserTokenPair issues an access+refresh pair bound to a user id.
func (s *JWTService) GenerateUserTokenPair(userID string) (*TokenPair, error) {
	now := time.Now()
	access, err := s.signUserToken(tokenTypeAccess, userID, now, s.accessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}
	refresh, err := s.signUserToken(tokenTypeRefresh, userID, now, s.refreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("sign refresh token: %w", err)
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(s.accessTokenTTL.Seconds()),
	}, nil
}

// GenerateUserAccessToken issues a fresh access token for a user id.
func (s *JWTService) GenerateUserAccessToken(userID string) (string, int64, error) {
	token, err := s.signUserToken(tokenTypeAccess, userID, time.Now(), s.accessTokenTTL)
	if err != nil {
		return "", 0, fmt.Errorf("sign access token: %w", err)
	}
	return token, int64(s.accessTokenTTL.Seconds()), nil
}

// ParseAccessToken validates an access token and returns its uid.
func (s *JWTService) ParseAccessToken(tokenStr string) (string, error) {
	return s.parseUserToken(tokenStr, tokenTypeAccess)
}

// ParseRefreshToken validates a refresh token and returns its uid.
func (s *JWTService) ParseRefreshToken(tokenStr string) (string, error) {
	return s.parseUserToken(tokenStr, tokenTypeRefresh)
}

func (s *JWTService) signUserToken(tokenType, userID string, now time.Time, ttl time.Duration) (string, error) {
	claims := Claims{
		Type: tokenType,
		UID:  userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *JWTService) parseUserToken(tokenStr, expectedType string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return "", fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || claims.Type != expectedType {
		return "", fmt.Errorf("invalid token type: expected %s", expectedType)
	}
	return claims.UID, nil
}
```

> Keep the existing `GenerateTokenPair` / `ValidateAccessToken` methods in place — `Refresh` (Task 9) and any remaining callers still compile. They simply won't carry a uid.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/infra/auth/ -run TestJWT_UserTokenRoundTrip -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/infra/auth/jwt.go internal/infra/auth/jwt_uid_test.go
git commit -m "feat(auth): embed and parse uid in JWT tokens"
```

---

### Task 9: Context helpers + middleware

**Files:**
- Create: `internal/infra/auth/context.go`
- Modify: `internal/infra/auth/middleware.go`
- Test: `internal/infra/auth/middleware_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/infra/auth/middleware_test.go`:

```go
package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type fakeUserLoader struct {
	status string
	err    error
}

func (f fakeUserLoader) UserStatus(uid string) (string, error) { return f.status, f.err }

func newTestContext(jwt *JWTService, loader UserStatusLoader, resolver *PermissionResolver, token string) (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/api")
	grp.Use(AuthMiddleware(jwt, loader))
	grp.GET("/secured", RequirePermission(resolver, PermWorkersRead), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	return r, rec
}

func TestAuthMiddleware_RejectsDisabledUser(t *testing.T) {
	jwt := NewJWTService("s", time.Hour, time.Hour)
	pair, _ := jwt.GenerateUserTokenPair("u1")
	loader := fakeUserLoader{status: "disabled"}
	resolver := NewPermissionResolver(func(string) ([]string, error) { return []string{"*"}, nil })

	r, rec := newTestContext(jwt, loader, resolver, pair.AccessToken)
	req := httptest.NewRequest(http.MethodGet, "/api/secured", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected disabled user rejected, got %d", rec.Code)
	}
}

func TestRequirePermission_AllowsAndDenies(t *testing.T) {
	jwt := NewJWTService("s", time.Hour, time.Hour)
	pair, _ := jwt.GenerateUserTokenPair("u1")
	loader := fakeUserLoader{status: "active"}

	// has permission
	resolverYes := NewPermissionResolver(func(string) ([]string, error) { return []string{PermWorkersRead}, nil })
	r, rec := newTestContext(jwt, loader, resolverYes, pair.AccessToken)
	req := httptest.NewRequest(http.MethodGet, "/api/secured", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// lacks permission
	resolverNo := NewPermissionResolver(func(string) ([]string, error) { return []string{PermTasksRead}, nil })
	r2, rec2 := newTestContext(jwt, loader, resolverNo, pair.AccessToken)
	req2 := httptest.NewRequest(http.MethodGet, "/api/secured", nil)
	req2.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec2.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/auth/ -run 'TestAuthMiddleware|TestRequirePermission' -v`
Expected: FAIL — `AuthMiddleware` / `UserStatusLoader` undefined.

- [ ] **Step 3: Implement context helpers**

Create `internal/infra/auth/context.go`:

```go
package auth

import "github.com/gin-gonic/gin"

const ctxUserIDKey = "auth_user_id"

// SetUserID stores the authenticated user id on the request context.
func SetUserID(c *gin.Context, uid string) { c.Set(ctxUserIDKey, uid) }

// UserID returns the authenticated user id, or "" if unauthenticated.
func UserID(c *gin.Context) string {
	v, ok := c.Get(ctxUserIDKey)
	if !ok {
		return ""
	}
	uid, _ := v.(string)
	return uid
}
```

- [ ] **Step 4: Replace middleware**

Replace the entire contents of `internal/infra/auth/middleware.go`:

```go
package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
)

// UserStatusLoader returns the account status for a user id.
type UserStatusLoader interface {
	UserStatus(userID string) (string, error)
}

// AuthMiddleware validates the access token, loads the user id, and rejects
// missing/disabled accounts.
func AuthMiddleware(jwtSvc *JWTService, users UserStatusLoader) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractBearerToken(c)
		if token == "" {
			token = c.Query("token")
		}
		uid, err := jwtSvc.ParseAccessToken(token)
		if err != nil || uid == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		status, err := users.UserStatus(uid)
		if err != nil || status != model.UserStatusActive {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		SetUserID(c, uid)
		c.Next()
	}
}

// RequirePermission aborts with 403 unless the current user holds perm.
func RequirePermission(resolver *PermissionResolver, perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := UserID(c)
		if uid == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		ok, err := resolver.HasPermission(uid, perm)
		if err != nil || !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}

func extractBearerToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return authHeader[7:]
	}
	return ""
}
```

> This removes the old `JWTMiddleware`. Task 14/15 update all references. `UserStore` (Task 4) must satisfy `UserStatusLoader` — add the method in Step 5.

- [ ] **Step 5: Add `UserStatus` to UserStore**

Append to `internal/infra/store/user_store.go`:

```go
// UserStatus returns a user's account status (implements auth.UserStatusLoader).
func (s *UserStore) UserStatus(userID string) (string, error) {
	var status string
	err := s.db.QueryRow(`SELECT status FROM bee_users WHERE id = ?`, userID).Scan(&status)
	return status, err
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/infra/auth/ -run 'TestAuthMiddleware|TestRequirePermission' -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/infra/auth/context.go internal/infra/auth/middleware.go internal/infra/auth/middleware_test.go internal/infra/store/user_store.go
git commit -m "feat(auth): user-aware middleware and RequirePermission guard"
```

---

## Phase C — Handlers & routes

### Task 10: Rewrite AuthHandler (login/refresh/me/change-password)

**Files:**
- Modify: `internal/infra/auth/handler.go`
- Test: `internal/api/auth_handler_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/api/auth_handler_test.go`:

```go
package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newAuthTestServer(t *testing.T) (*gin.Engine, *store.UserStore, *auth.JWTService) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	us := store.NewUserStore(db)
	if _, err := us.Create("alice", "s3cret", "Alice", "", []string{model.RoleIDAdmin}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	jwtSvc := auth.NewJWTService("secret", time.Hour, 24*time.Hour)
	rl := auth.NewLoginRateLimiter(50, time.Minute)
	h := auth.NewAuthHandler(us, jwtSvc, rl)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/auth/login", h.Login)
	return r, us, jwtSvc
}

func TestAuthHandler_LoginSuccess(t *testing.T) {
	r, _, _ := newAuthTestServer(t)
	body, _ := json.Marshal(map[string]string{"username": "alice", "password": "s3cret"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var pair auth.TokenPair
	_ = json.Unmarshal(rec.Body.Bytes(), &pair)
	if pair.AccessToken == "" {
		t.Fatal("expected access token")
	}
}

func TestAuthHandler_LoginBadPassword(t *testing.T) {
	r, _, _ := newAuthTestServer(t)
	body, _ := json.Marshal(map[string]string{"username": "alice", "password": "nope"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestAuthHandler -v`
Expected: FAIL — `NewAuthHandler` signature mismatch (old one takes username/password strings).

- [ ] **Step 3: Rewrite handler**

Replace the entire contents of `internal/infra/auth/handler.go`:

```go
package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
)

// UserAuthenticator authenticates credentials and loads user data.
type UserAuthenticator interface {
	Authenticate(username, password string) (model.UserWithRoles, error)
	GetByID(id string) (model.UserWithRoles, error)
	SetPassword(id, plainPassword string) error
}

type AuthHandler struct {
	users       UserAuthenticator
	jwtSvc      *JWTService
	rateLimiter *LoginRateLimiter
	resolver    *PermissionResolver
}

func NewAuthHandler(users UserAuthenticator, jwtSvc *JWTService, rateLimiter *LoginRateLimiter) *AuthHandler {
	return &AuthHandler{users: users, jwtSvc: jwtSvc, rateLimiter: rateLimiter}
}

// WithResolver attaches the permission resolver (needed for /api/me). Returns the
// handler for chaining.
func (h *AuthHandler) WithResolver(r *PermissionResolver) *AuthHandler {
	h.resolver = r
	return h
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	if !h.rateLimiter.Allow(c.ClientIP()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many attempts, please try again later"})
		return
	}
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	user, err := h.users.Authenticate(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}
	pair, err := h.jwtSvc.GenerateUserTokenPair(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	c.JSON(http.StatusOK, pair)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type refreshResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	uid, err := h.jwtSvc.ParseRefreshToken(req.RefreshToken)
	if err != nil || uid == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
		return
	}
	accessToken, expiresIn, err := h.jwtSvc.GenerateUserAccessToken(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	c.JSON(http.StatusOK, refreshResponse{AccessToken: accessToken, ExpiresIn: expiresIn})
}

type meResponse struct {
	model.UserWithRoles
	Permissions []string `json:"permissions"`
}

// Me returns the current user with resolved permissions.
func (h *AuthHandler) Me(c *gin.Context) {
	uid := UserID(c)
	user, err := h.users.GetByID(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	perms, err := h.resolver.PermissionsFor(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve permissions"})
		return
	}
	c.JSON(http.StatusOK, meResponse{UserWithRoles: user, Permissions: perms})
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// ChangePassword updates the current user's password after verifying the old one.
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	uid := UserID(c)
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.users.GetByID(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if _, err := h.users.Authenticate(user.Username, req.OldPassword); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "old password is incorrect"})
		return
	}
	if err := h.users.SetPassword(uid, req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update password"})
		return
	}
	c.Status(http.StatusNoContent)
}
```

> `UserStore` already has `Authenticate`, `GetByID`, `SetPassword` — it satisfies `UserAuthenticator`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestAuthHandler -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/infra/auth/handler.go internal/api/auth_handler_test.go
git commit -m "feat(auth): login against UserStore, add Me and ChangePassword"
```

---

### Task 11: Setup handler (wizard)

**Files:**
- Create: `internal/api/setup_handler.go`
- Test: `internal/api/setup_handler_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/api/setup_handler_test.go`:

```go
package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/api"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newSetupServer(t *testing.T) (*gin.Engine, *store.UserStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	us := store.NewUserStore(db)
	jwtSvc := auth.NewJWTService("secret", time.Hour, time.Hour)
	h := api.NewSetupHandler(us, jwtSvc)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/setup/status", h.Status)
	r.POST("/api/setup", h.Create)
	return r, us
}

func TestSetup_StatusFalseThenTrue(t *testing.T) {
	r, _ := newSetupServer(t)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/setup/status", nil))
	var resp map[string]bool
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["initialized"] {
		t.Fatal("expected uninitialized")
	}

	body, _ := json.Marshal(map[string]string{"username": "root", "password": "rootpw"})
	req := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec2.Code, rec2.Body.String())
	}

	rec3 := httptest.NewRecorder()
	r.ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/api/setup/status", nil))
	_ = json.Unmarshal(rec3.Body.Bytes(), &resp)
	if !resp["initialized"] {
		t.Fatal("expected initialized after create")
	}
}

func TestSetup_SecondCreateRejected(t *testing.T) {
	r, _ := newSetupServer(t)
	body, _ := json.Marshal(map[string]string{"username": "root", "password": "rootpw"})
	for i := 0; i < 1; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
	}
	// second attempt
	req := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on re-init, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestSetup -v`
Expected: FAIL — `NewSetupHandler` undefined.

- [ ] **Step 3: Implement**

Create `internal/api/setup_handler.go`:

```go
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type SetupHandler struct {
	users  *store.UserStore
	jwtSvc *auth.JWTService
}

func NewSetupHandler(users *store.UserStore, jwtSvc *auth.JWTService) *SetupHandler {
	return &SetupHandler{users: users, jwtSvc: jwtSvc}
}

// Status reports whether the system already has at least one user.
func (h *SetupHandler) Status(c *gin.Context) {
	n, err := h.users.Count()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"initialized": n > 0})
}

type setupRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required,min=6"`
	DisplayName string `json:"display_name"`
}

// Create provisions the first super-admin. Only works while no users exist.
func (h *SetupHandler) Create(c *gin.Context) {
	n, err := h.users.Count()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "system already initialized"})
		return
	}
	var req setupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.users.Create(req.Username, req.Password, req.DisplayName, "", []string{model.RoleIDSuperAdmin})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	pair, err := h.jwtSvc.GenerateUserTokenPair(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	c.JSON(http.StatusOK, pair)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestSetup -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/setup_handler.go internal/api/setup_handler_test.go
git commit -m "feat(api): add setup wizard handler with one-time guard"
```

---

### Task 12: User management handler

**Files:**
- Create: `internal/api/user_handler.go`
- Test: `internal/api/user_handler_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/api/user_handler_test.go`:

```go
package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/api"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newUserServer(t *testing.T) (*gin.Engine, *store.UserStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	us := store.NewUserStore(db)
	resolver := auth.NewPermissionResolver(us.PermissionsForUser)
	h := api.NewUserHandler(us, resolver)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/users", h.List)
	r.POST("/api/users", h.Create)
	r.PUT("/api/users/:id/roles", h.SetRoles)
	r.PUT("/api/users/:id/status", h.SetStatus)
	r.POST("/api/users/:id/password", h.ResetPassword)
	r.DELETE("/api/users/:id", h.Delete)
	return r, us
}

func TestUserHandler_CreateAndList(t *testing.T) {
	r, _ := newUserServer(t)
	body, _ := json.Marshal(map[string]any{
		"username": "bob", "password": "bobpw", "display_name": "Bob",
		"role_ids": []string{model.RoleIDMember},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/users", nil))
	var users []model.UserWithRoles
	_ = json.Unmarshal(rec2.Body.Bytes(), &users)
	if len(users) != 1 || users[0].Username != "bob" {
		t.Fatalf("unexpected users: %+v", users)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestUserHandler -v`
Expected: FAIL — `NewUserHandler` undefined.

- [ ] **Step 3: Implement**

Create `internal/api/user_handler.go`:

```go
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type UserHandler struct {
	users    *store.UserStore
	resolver *auth.PermissionResolver
}

func NewUserHandler(users *store.UserStore, resolver *auth.PermissionResolver) *UserHandler {
	return &UserHandler{users: users, resolver: resolver}
}

func (h *UserHandler) List(c *gin.Context) {
	users, err := h.users.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if users == nil {
		users = []model.UserWithRoles{}
	}
	c.JSON(http.StatusOK, users)
}

type createUserRequest struct {
	Username    string   `json:"username" binding:"required"`
	Password    string   `json:"password" binding:"required,min=6"`
	DisplayName string   `json:"display_name"`
	RoleIDs     []string `json:"role_ids"`
}

func (h *UserHandler) Create(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.users.Create(req.Username, req.Password, req.DisplayName, auth.UserID(c), req.RoleIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, user)
}

type setRolesRequest struct {
	RoleIDs []string `json:"role_ids"`
}

func (h *UserHandler) SetRoles(c *gin.Context) {
	id := c.Param("id")
	var req setRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.guardLastSuperAdmin(c, id, req.RoleIDs); err != nil {
		return
	}
	if err := h.users.SetRoles(id, req.RoleIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.resolver.Invalidate(id)
	c.Status(http.StatusNoContent)
}

type setStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active disabled"`
}

func (h *UserHandler) SetStatus(c *gin.Context) {
	id := c.Param("id")
	var req setStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Status == model.UserStatusDisabled {
		if err := h.guardLastSuperAdmin(c, id, nil); err != nil {
			return
		}
	}
	if err := h.users.SetStatus(id, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.resolver.Invalidate(id)
	c.Status(http.StatusNoContent)
}

type resetPasswordRequest struct {
	Password string `json:"password" binding:"required,min=6"`
}

func (h *UserHandler) ResetPassword(c *gin.Context) {
	id := c.Param("id")
	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.users.SetPassword(id, req.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *UserHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.guardLastSuperAdmin(c, id, nil); err != nil {
		return
	}
	if err := h.users.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.resolver.Invalidate(id)
	c.Status(http.StatusNoContent)
}

// guardLastSuperAdmin blocks removing the super-admin role from / disabling /
// deleting the last remaining active super-admin. newRoleIDs is the prospective
// role set for a SetRoles call, or nil for disable/delete.
func (h *UserHandler) guardLastSuperAdmin(c *gin.Context, userID string, newRoleIDs []string) error {
	user, err := h.users.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return err
	}
	isSuper := false
	for _, r := range user.Roles {
		if r.ID == model.RoleIDSuperAdmin {
			isSuper = true
		}
	}
	if !isSuper {
		return nil
	}
	// If newRoleIDs still grants super-admin, the change is safe.
	for _, rid := range newRoleIDs {
		if rid == model.RoleIDSuperAdmin {
			return nil
		}
	}
	count, err := h.users.CountActiveSuperAdmins()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return err
	}
	if count <= 1 {
		err := errLastSuperAdmin
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return err
	}
	return nil
}
```

Add the sentinel error and the count method. Append to `internal/api/user_handler.go`:

```go
import "errors"

var errLastSuperAdmin = errors.New("cannot remove or disable the last active super-admin")
```

> Place the `errors` import in the existing import block instead of a second `import` statement.

Append to `internal/infra/store/user_store.go`:

```go
// CountActiveSuperAdmins counts active users holding the super-admin role.
func (s *UserStore) CountActiveSuperAdmins() (int, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(DISTINCT u.id)
		FROM bee_users u
		JOIN bee_user_roles ur ON ur.user_id = u.id
		WHERE ur.role_id = ? AND u.status = ?`,
		model.RoleIDSuperAdmin, model.UserStatusActive,
	).Scan(&n)
	return n, err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestUserHandler -v`
Expected: PASS

- [ ] **Step 5: Add a guard test**

Append to `internal/api/user_handler_test.go`:

```go
func TestUserHandler_CannotDeleteLastSuperAdmin(t *testing.T) {
	r, us := newUserServer(t)
	su, _ := us.Create("root", "rootpw", "Root", "", []string{model.RoleIDSuperAdmin})
	req := httptest.NewRequest(http.MethodDelete, "/api/users/"+su.ID, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 deleting last super-admin, got %d", rec.Code)
	}
}
```

Run: `go test ./internal/api/ -run TestUserHandler -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/api/user_handler.go internal/api/user_handler_test.go internal/infra/store/user_store.go
git commit -m "feat(api): user management endpoints with last-super-admin guard"
```

---

### Task 13: Role management + permission catalog handler

**Files:**
- Create: `internal/api/role_handler.go`
- Test: `internal/api/role_handler_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/api/role_handler_test.go`:

```go
package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/api"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newRoleServer(t *testing.T) (*gin.Engine, *store.RoleStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	rs := store.NewRoleStore(db)
	resolver := auth.NewPermissionResolver(func(string) ([]string, error) { return nil, nil })
	h := api.NewRoleHandler(rs, resolver)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/permissions", h.Catalog)
	r.GET("/api/roles", h.List)
	r.POST("/api/roles", h.Create)
	r.PUT("/api/roles/:id", h.Update)
	r.DELETE("/api/roles/:id", h.Delete)
	return r, rs
}

func TestRoleHandler_Catalog(t *testing.T) {
	r, _ := newRoleServer(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/permissions", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var groups []auth.PermissionGroup
	_ = json.Unmarshal(rec.Body.Bytes(), &groups)
	if len(groups) == 0 {
		t.Fatal("expected non-empty catalog")
	}
}

func TestRoleHandler_CreateAndDelete(t *testing.T) {
	r, _ := newRoleServer(t)
	body, _ := json.Marshal(map[string]any{
		"name": "ops", "permissions": []string{"workers:read"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/roles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var role model.RoleWithPermissions
	_ = json.Unmarshal(rec.Body.Bytes(), &role)

	del := httptest.NewRequest(http.MethodDelete, "/api/roles/"+role.ID, nil)
	recDel := httptest.NewRecorder()
	r.ServeHTTP(recDel, del)
	if recDel.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", recDel.Code)
	}
}

func TestRoleHandler_CannotDeleteSystemRole(t *testing.T) {
	r, _ := newRoleServer(t)
	del := httptest.NewRequest(http.MethodDelete, "/api/roles/"+model.RoleIDSuperAdmin, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, del)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 deleting system role, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestRoleHandler -v`
Expected: FAIL — `NewRoleHandler` undefined.

- [ ] **Step 3: Implement**

Create `internal/api/role_handler.go`:

```go
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type RoleHandler struct {
	roles    *store.RoleStore
	resolver *auth.PermissionResolver
}

func NewRoleHandler(roles *store.RoleStore, resolver *auth.PermissionResolver) *RoleHandler {
	return &RoleHandler{roles: roles, resolver: resolver}
}

// Catalog returns the grouped permission catalog.
func (h *RoleHandler) Catalog(c *gin.Context) {
	c.JSON(http.StatusOK, auth.PermissionCatalog())
}

func (h *RoleHandler) List(c *gin.Context) {
	roles, err := h.roles.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if roles == nil {
		roles = []model.RoleWithPermissions{}
	}
	c.JSON(http.StatusOK, roles)
}

type roleRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func (h *RoleHandler) Create(c *gin.Context) {
	var req roleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validatePermissions(req.Permissions); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	role, err := h.roles.Create(model.Role{Name: req.Name, Description: req.Description}, req.Permissions)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, role)
}

func (h *RoleHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var req roleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validatePermissions(req.Permissions); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.roles.Update(
		model.Role{ID: id, Name: req.Name, Description: req.Description}, req.Permissions,
	); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.resolver.InvalidateAll() // role permission change affects every member
	c.Status(http.StatusNoContent)
}

func (h *RoleHandler) Delete(c *gin.Context) {
	if err := h.roles.Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.resolver.InvalidateAll()
	c.Status(http.StatusNoContent)
}

// validatePermissions rejects unknown permission keys (wildcard not assignable here).
func validatePermissions(perms []string) error {
	valid := map[string]struct{}{}
	for _, p := range auth.AllPermissions() {
		valid[p] = struct{}{}
	}
	for _, p := range perms {
		if _, ok := valid[p]; !ok {
			return errUnknownPermission(p)
		}
	}
	return nil
}
```

Add the error helper. Append to `internal/api/role_handler.go` (and add `"fmt"` to the import block):

```go
func errUnknownPermission(p string) error {
	return fmt.Errorf("unknown permission: %s", p)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestRoleHandler -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/role_handler.go internal/api/role_handler_test.go
git commit -m "feat(api): role management and permission catalog endpoints"
```

---

### Task 14: Route registration + per-route permissions

**Files:**
- Modify: `internal/routes/server.go`
- Modify: `internal/routes/api.go`

- [ ] **Step 1: Update ServerParams**

In `internal/routes/server.go`, replace the `JWTMiddleware gin.HandlerFunc` field and add new fields. The `ServerParams` struct becomes:

```go
type ServerParams struct {
	Workers           *api.WorkerHandler
	Executions        *api.ExecutionHandler
	Messages          *api.MessageHandler
	Tasks             *api.TaskHandler
	Departments       *api.DepartmentHandler
	Stats             *api.StatsHandler
	Config            *api.ConfigHandler
	Version           *api.VersionHandler
	LocalChat         *api.LocalChatHandler
	Auth              *auth.AuthHandler
	Envs              *api.EnvHandler
	SystemConfigs     *api.SystemConfigHandler
	Users             *api.UserHandler
	Roles             *api.RoleHandler
	Setup             *api.SetupHandler
	BeeRPC            *rpc.Server
	RPCAuthMiddleware gin.HandlerFunc
	StaticFS          fs.FS
	AuthMiddleware    gin.HandlerFunc
	Resolver          *auth.PermissionResolver
}
```

- [ ] **Step 2: Update setupRoutes for the setup endpoints**

In `internal/routes/server.go`, replace `setupRoutes`:

```go
func (s *Server) setupRoutes() error {
	s.registerAuthRoutes()
	s.router.GET("/api/config", s.Config.Get)
	s.router.GET("/api/setup/status", s.Setup.Status)
	s.router.POST("/api/setup", s.Setup.Create)

	apiGroup := s.router.Group("/api")
	apiGroup.Use(s.AuthMiddleware)
	s.registerAPIRoutes(apiGroup)

	s.registerRPCRoutes()

	return s.registerStaticRoutes()
}
```

- [ ] **Step 3: Update api.go with permission guards**

Replace the entire contents of `internal/routes/api.go`:

```go
package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/auth"
)

func (s *Server) registerAuthRoutes() {
	authGroup := s.router.Group("/api/auth")
	authGroup.POST("/login", s.Auth.Login)
	authGroup.POST("/refresh", s.Auth.Refresh)
}

func (s *Server) registerAPIRoutes(r *gin.RouterGroup) {
	rp := func(perm string) gin.HandlerFunc { return auth.RequirePermission(s.Resolver, perm) }

	// Current-user endpoints (any authenticated user)
	r.GET("/me", s.Auth.Me)
	r.POST("/me/password", s.Auth.ChangePassword)

	// Workers
	r.POST("/workers", rp(auth.PermWorkersWrite), s.Workers.Create)
	r.GET("/workers", rp(auth.PermWorkersRead), s.Workers.List)
	r.GET("/workers/random-name", rp(auth.PermWorkersRead), s.Workers.RandomName)
	r.GET("/workers/:id", rp(auth.PermWorkersRead), s.Workers.Get)
	r.PUT("/workers/:id", rp(auth.PermWorkersWrite), s.Workers.Update)
	r.DELETE("/workers/:id", rp(auth.PermWorkersWrite), s.Workers.Delete)

	// Sessions
	r.GET("/sessions", rp(auth.PermSessionsRead), s.Executions.List)
	r.GET("/sessions/:id", rp(auth.PermSessionsRead), s.Executions.GetSession)
	r.GET("/sessions/:id/logs", rp(auth.PermSessionsRead), s.Executions.GetLogs)

	// Tasks
	r.GET("/tasks", rp(auth.PermTasksRead), s.Tasks.List)
	r.DELETE("/tasks/:id", rp(auth.PermTasksWrite), s.Tasks.Cancel)
	r.POST("/workers/:id/tasks/cancel-all", rp(auth.PermTasksWrite), s.Tasks.CancelByWorker)

	// Departments
	r.POST("/departments", rp(auth.PermDepartmentsWrite), s.Departments.Create)
	r.GET("/departments", rp(auth.PermDepartmentsRead), s.Departments.List)
	r.GET("/departments/:id", rp(auth.PermDepartmentsRead), s.Departments.Get)
	r.PUT("/departments/:id", rp(auth.PermDepartmentsWrite), s.Departments.Update)
	r.DELETE("/departments/:id", rp(auth.PermDepartmentsWrite), s.Departments.Delete)
	r.PUT("/workers/:id/departments", rp(auth.PermDepartmentsWrite), s.Departments.SetWorkerDepartments)
	r.GET("/workers/:id/departments", rp(auth.PermDepartmentsRead), s.Departments.GetWorkerDepartments)
	r.GET("/departments/:id/workers", rp(auth.PermDepartmentsRead), s.Departments.GetDepartmentWorkers)

	// Local chat (treated as worker interaction)
	r.POST("/local/messages", rp(auth.PermWorkersWrite), s.LocalChat.SendMessage)
	r.GET("/local/messages", rp(auth.PermWorkersRead), s.LocalChat.GetMessages)
	r.POST("/local/media", rp(auth.PermWorkersWrite), s.LocalChat.UploadMedia)
	r.GET("/local/media/:filename", rp(auth.PermWorkersRead), s.LocalChat.ServeMedia)
	r.GET("/local/stream", rp(auth.PermWorkersRead), s.LocalChat.StreamReplies)

	// Messages
	r.GET("/messages", rp(auth.PermMessagesRead), s.Messages.List)

	// Version (any authenticated user)
	r.GET("/version", s.Version.Get)

	// Stats
	r.GET("/stats/overview", rp(auth.PermStatsRead), s.Stats.GetOverview)
	r.GET("/stats/token-trend", rp(auth.PermStatsRead), s.Stats.GetTokenTrend)

	// Env
	r.GET("/envs", rp(auth.PermEnvRead), s.Envs.List)
	r.POST("/envs", rp(auth.PermEnvWrite), s.Envs.Create)
	r.PUT("/envs/:id", rp(auth.PermEnvWrite), s.Envs.Update)
	r.DELETE("/envs/:id", rp(auth.PermEnvWrite), s.Envs.Delete)

	// System configs
	r.GET("/system-configs", rp(auth.PermSystemConfigRead), s.SystemConfigs.Get)
	r.PUT("/system-configs/:key", rp(auth.PermSystemConfigWrite), s.SystemConfigs.Set)

	// User & role administration
	r.GET("/users", rp(auth.PermUsersManage), s.Users.List)
	r.POST("/users", rp(auth.PermUsersManage), s.Users.Create)
	r.PUT("/users/:id/roles", rp(auth.PermUsersManage), s.Users.SetRoles)
	r.PUT("/users/:id/status", rp(auth.PermUsersManage), s.Users.SetStatus)
	r.POST("/users/:id/password", rp(auth.PermUsersManage), s.Users.ResetPassword)
	r.DELETE("/users/:id", rp(auth.PermUsersManage), s.Users.Delete)

	r.GET("/permissions", rp(auth.PermRolesManage), s.Roles.Catalog)
	r.GET("/roles", rp(auth.PermRolesManage), s.Roles.List)
	r.POST("/roles", rp(auth.PermRolesManage), s.Roles.Create)
	r.PUT("/roles/:id", rp(auth.PermRolesManage), s.Roles.Update)
	r.DELETE("/roles/:id", rp(auth.PermRolesManage), s.Roles.Delete)
}
```

- [ ] **Step 4: Verify it compiles (wiring completes in Task 15)**

Run: `go build ./internal/routes/`
Expected: FAIL — `ServerParams` constructed in `app.go` still uses old fields. This is fixed in Task 15; do not commit yet.

- [ ] **Step 5: (No commit — proceed to Task 15, commit together)**

---

### Task 15: App wiring

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/infra/config/config.go` (deprecation comment only)

- [ ] **Step 1: Add stores to appStores**

In `internal/app/app.go`, add fields to the `appStores` struct:

```go
	userStore         *store.UserStore
	roleStore         *store.RoleStore
```

And in `buildStores`'s returned `appStores{...}` literal, add:

```go
		userStore:         store.NewUserStore(db),
		roleStore:         store.NewRoleStore(db),
```

- [ ] **Step 2: Rewire buildAPIServer**

In `internal/app/app.go`, replace the body of `buildAPIServer` (the part that builds jwt/auth/middleware and constructs `routes.ServerParams`):

```go
	secret := serverCfg.Auth.JWTSecret
	jwtSvc := auth.NewJWTService(secret, serverCfg.Auth.AccessTokenTTL, serverCfg.Auth.RefreshTokenTTL)
	rateLimiter := auth.NewLoginRateLimiter(5, time.Minute)
	resolver := auth.NewPermissionResolver(s.userStore.PermissionsForUser)
	authHandler := auth.NewAuthHandler(s.userStore, jwtSvc, rateLimiter).WithResolver(resolver)
	authMiddleware := auth.AuthMiddleware(jwtSvc, s.userStore)
	rpcAuthMiddleware := rpc.JWTAuthMiddleware(rpcCfg.TokenSecret)

	return routes.NewServer(routes.ServerParams{
		Workers:           api.NewWorkerHandler(s.workerStore, s.departmentStore, mgr, language),
		Executions:        api.NewExecutionHandler(s.execStore, s.tokenStatsStore),
		Messages:          api.NewMessageHandler(s.msgStore),
		Tasks:             api.NewTaskHandler(s.taskStore, s.workerStore, taskCanceller),
		Departments:       api.NewDepartmentHandler(s.departmentStore, s.workerStore),
		Stats:             api.NewStatsHandler(s.statsStore),
		Config:            api.NewConfigHandler(language, mgr.EnabledEngines()),
		Version:           api.NewVersionHandler(buildinfo.Get()),
		LocalChat:         localChat,
		Auth:              authHandler,
		Envs:              api.NewEnvHandler(envSvc),
		SystemConfigs:     api.NewSystemConfigHandler(s.systemConfigStore, mgr, engineCfg),
		Users:             api.NewUserHandler(s.userStore, resolver),
		Roles:             api.NewRoleHandler(s.roleStore, resolver),
		Setup:             api.NewSetupHandler(s.userStore, jwtSvc),
		BeeRPC:            beeRPCSrv,
		RPCAuthMiddleware: rpcAuthMiddleware,
		StaticFS:          webui.DistFS,
		AuthMiddleware:    authMiddleware,
		Resolver:          resolver,
	})
```

- [ ] **Step 3: Mark config fields deprecated**

In `internal/infra/config/config.go`, update the `AuthConfig` comments for `Username`/`Password`:

```go
	Username        string        `yaml:"username"`          // DEPRECATED: web login now uses DB users; ignored for login
	Password        string        `yaml:"password"`          // DEPRECATED: web login now uses DB users; ignored for login
```

Leave the `applyDefaults` logic untouched (the fields still parse harmlessly).

- [ ] **Step 4: Build the whole backend**

Run: `go build ./...`
Expected: success (no output). If there is a leftover reference to `auth.JWTMiddleware` or `auth.NewAuthHandler(username, password, ...)`, fix it to the new API.

- [ ] **Step 5: Run the full backend test suite**

Run: `go test ./internal/...`
Expected: PASS (all packages)

- [ ] **Step 6: Commit Tasks 14 + 15 together**

```bash
git add internal/routes/server.go internal/routes/api.go internal/app/app.go internal/infra/config/config.go
git commit -m "feat(app): wire RBAC stores, handlers, resolver, and per-route permissions"
```

---

## Phase D — Frontend

> Mirror existing pages for styling and data-fetching conventions. Read `web/src/lib/auth.ts`, `web/src/pages/login.tsx`, `web/src/pages/departments.tsx`, `web/src/components/auth-guard.tsx`, and `web/src/app.tsx` before starting. All new components use radius ≤ `sm` (`rounded-none` / `rounded-sm` / `rounded-full` only).

### Task 16: Frontend permission helper + auth/me integration

**Files:**
- Create: `web/src/lib/permissions.ts`
- Modify: `web/src/lib/auth.ts`

- [ ] **Step 1: Create permission keys + helper**

Create `web/src/lib/permissions.ts`:

```ts
export const Perm = {
  workersRead: "workers:read",
  workersWrite: "workers:write",
  tasksRead: "tasks:read",
  tasksWrite: "tasks:write",
  departmentsRead: "departments:read",
  departmentsWrite: "departments:write",
  messagesRead: "messages:read",
  sessionsRead: "sessions:read",
  sessionsWrite: "sessions:write",
  statsRead: "stats:read",
  envRead: "env:read",
  envWrite: "env:write",
  systemConfigRead: "system_config:read",
  systemConfigWrite: "system_config:write",
  usersManage: "users:manage",
  rolesManage: "roles:manage",
} as const;

export type PermissionKey = (typeof Perm)[keyof typeof Perm];

export function hasPermission(perms: string[] | undefined, key: string): boolean {
  if (!perms) return false;
  return perms.includes("*") || perms.includes(key);
}
```

- [ ] **Step 2: Read current auth.ts**

Run: `sed -n '1,200p' web/src/lib/auth.ts`
Identify: token storage keys, the fetch wrapper, and any existing `getToken`/`logout` exports.

- [ ] **Step 3: Add current-user state to auth.ts**

Add to `web/src/lib/auth.ts` (adapt names to match the file's existing fetch helper and token accessor):

```ts
export interface Role {
  id: string;
  name: string;
  description: string;
  is_system: boolean;
}

export interface CurrentUser {
  id: string;
  username: string;
  display_name: string;
  status: string;
  roles: Role[];
  permissions: string[];
}

export async function fetchMe(): Promise<CurrentUser> {
  const res = await fetch("/api/me", {
    headers: { Authorization: `Bearer ${getToken()}` },
  });
  if (!res.ok) throw new Error("failed to load current user");
  return res.json();
}

export async function fetchSetupStatus(): Promise<boolean> {
  const res = await fetch("/api/setup/status");
  if (!res.ok) return true; // fail safe: assume initialized
  const data = await res.json();
  return Boolean(data.initialized);
}
```

> `getToken` must be the existing token accessor in this file — reuse it, do not invent a new one. If the file uses an axios-style client instead of `fetch`, route these calls through that client.

- [ ] **Step 4: Verify build**

Run: `cd web && npm run build` (or the project's typecheck script — check `web/package.json` scripts first)
Expected: success

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/permissions.ts web/src/lib/auth.ts
git commit -m "feat(web): add permission helper and /api/me integration"
```

---

### Task 17: Setup wizard page + guard

**Files:**
- Create: `web/src/pages/setup.tsx`
- Modify: `web/src/components/auth-guard.tsx`
- Modify: `web/src/app.tsx`

- [ ] **Step 1: Read existing patterns**

Run: `sed -n '1,200p' web/src/pages/login.tsx` and `sed -n '1,120p' web/src/components/auth-guard.tsx` and `sed -n '1,160p' web/src/app.tsx`
Note: how routes are declared, how login stores tokens and redirects, the form/input components used.

- [ ] **Step 2: Create the setup page**

Create `web/src/pages/setup.tsx`, mirroring `login.tsx`'s structure (same input components, same token-storing flow), but POSTing to `/api/setup`:

```tsx
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { setTokens } from "../lib/auth"; // use the actual token-storing export from login.tsx

export default function SetupPage() {
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError("");
    try {
      const res = await fetch("/api/setup", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password, display_name: displayName }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || "setup failed");
      }
      const pair = await res.json();
      setTokens(pair); // reuse login.tsx's token persistence
      navigate("/", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "setup failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center">
      <form onSubmit={onSubmit} className="w-full max-w-sm space-y-4 rounded-sm border p-6">
        <h1 className="text-lg font-semibold">Create super-admin</h1>
        <input className="w-full rounded-sm border px-3 py-2" placeholder="Username"
          value={username} onChange={(e) => setUsername(e.target.value)} required />
        <input className="w-full rounded-sm border px-3 py-2" placeholder="Display name"
          value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
        <input className="w-full rounded-sm border px-3 py-2" type="password" placeholder="Password (min 6)"
          value={password} onChange={(e) => setPassword(e.target.value)} required minLength={6} />
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button className="w-full rounded-sm border px-3 py-2" disabled={loading} type="submit">
          {loading ? "Creating…" : "Create"}
        </button>
      </form>
    </div>
  );
}
```

> Replace `setTokens` and the raw inputs with the exact helpers/components used by `login.tsx`. Keep all radii at `rounded-sm`.

- [ ] **Step 3: Add the setup-status gate**

In `web/src/components/auth-guard.tsx`, before the existing auth check, query setup status once and redirect to `/setup` when uninitialized. Add near the top of the guard component:

```tsx
import { fetchSetupStatus } from "../lib/auth";
// ...
const [initialized, setInitialized] = useState<boolean | null>(null);
useEffect(() => {
  fetchSetupStatus().then(setInitialized).catch(() => setInitialized(true));
}, []);
if (initialized === null) return null; // or existing loading spinner
if (!initialized) return <Navigate to="/setup" replace />;
```

> Use the router primitives already imported in this file (`Navigate`, `useEffect`, `useState`). The `/setup` route itself must NOT be wrapped by the guard.

- [ ] **Step 4: Register the route**

In `web/src/app.tsx`, add a public route for `/setup` outside the guarded area, mirroring how `/login` is registered:

```tsx
<Route path="/setup" element={<SetupPage />} />
```

Add the import: `import SetupPage from "./pages/setup";`

- [ ] **Step 5: Verify build**

Run: `cd web && npm run build`
Expected: success

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/setup.tsx web/src/components/auth-guard.tsx web/src/app.tsx
git commit -m "feat(web): setup wizard page and uninitialized redirect"
```

---

### Task 18: User management page

**Files:**
- Create: `web/src/pages/users.tsx`
- Modify: `web/src/app.tsx` (route)

- [ ] **Step 1: Read a CRUD page for the pattern**

Run: `sed -n '1,250p' web/src/pages/departments.tsx`
Note: the data-fetching hook, table/list rendering, create/edit modal pattern, and the authenticated fetch wrapper used.

- [ ] **Step 2: Create the page**

Create `web/src/pages/users.tsx` implementing:
- `GET /api/users` → list (username, display name, roles, status).
- "New user" form → `POST /api/users` with `{username, password, display_name, role_ids}`. Roles chosen from `GET /api/roles` (multi-select checkboxes).
- Per-row actions: enable/disable → `PUT /api/users/:id/status`; reset password → `POST /api/users/:id/password`; edit roles → `PUT /api/users/:id/roles`; delete → `DELETE /api/users/:id`.
- Surface backend 400 errors (e.g. last super-admin guard) inline.

Use the same authenticated fetch wrapper and table/modal components as `departments.tsx`. All radii `rounded-sm`; status pills may use `rounded-full`. Mirror this fetch shape (adapt to the project's wrapper):

```tsx
async function loadUsers(): Promise<UserWithRoles[]> {
  const res = await fetch("/api/users", { headers: { Authorization: `Bearer ${getToken()}` } });
  if (!res.ok) throw new Error("failed to load users");
  return res.json();
}
```

Define the local types:

```tsx
interface Role { id: string; name: string; is_system: boolean }
interface UserWithRoles {
  id: string; username: string; display_name: string; status: string; roles: Role[];
}
```

- [ ] **Step 3: Register the route (guarded + permission-gated)**

In `web/src/app.tsx`, add inside the guarded routes:

```tsx
<Route path="/users" element={<UsersPage />} />
```

Add `import UsersPage from "./pages/users";`

- [ ] **Step 4: Verify build**

Run: `cd web && npm run build`
Expected: success

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/users.tsx web/src/app.tsx
git commit -m "feat(web): user management page"
```

---

### Task 19: Role management page

**Files:**
- Create: `web/src/pages/roles.tsx`
- Modify: `web/src/app.tsx` (route)

- [ ] **Step 1: Create the page**

Create `web/src/pages/roles.tsx` implementing:
- `GET /api/roles` → list roles with their permissions; `GET /api/permissions` → grouped catalog for the checkbox UI.
- Create role → `POST /api/roles` `{name, description, permissions}`.
- Edit role → `PUT /api/roles/:id`. For `is_system` roles, render read-only (no edit/delete); for `super-admin` show all permissions checked and disabled.
- Delete custom role → `DELETE /api/roles/:id`; surface the 400 for system roles.

Define local types:

```tsx
interface PermissionGroup { resource: string; permissions: string[] }
interface RoleWithPermissions {
  id: string; name: string; description: string; is_system: boolean; permissions: string[];
}
```

Mirror the catalog fetch:

```tsx
async function loadCatalog(): Promise<PermissionGroup[]> {
  const res = await fetch("/api/permissions", { headers: { Authorization: `Bearer ${getToken()}` } });
  if (!res.ok) throw new Error("failed to load permission catalog");
  return res.json();
}
```

Render each group as a fieldset of checkboxes (radius `rounded-sm`). Reuse the table/modal/components from `users.tsx`/`departments.tsx`.

- [ ] **Step 2: Register the route**

In `web/src/app.tsx`, add inside the guarded routes:

```tsx
<Route path="/roles" element={<RolesPage />} />
```

Add `import RolesPage from "./pages/roles";`

- [ ] **Step 3: Verify build**

Run: `cd web && npm run build`
Expected: success

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/roles.tsx web/src/app.tsx
git commit -m "feat(web): role management page with permission catalog"
```

---

### Task 20: Permission-gated navigation + app bootstrap

**Files:**
- Modify: navigation component (find it: `grep -rln "to=\"/departments\"\|/sessions" web/src/components`)
- Modify: `web/src/app.tsx` or wherever the current user is loaded after login

- [ ] **Step 1: Load current user on app start**

After login/token presence, call `fetchMe()` once and store the result in the app's auth context/state (mirror how the app currently holds auth state — check `web/src/lib/auth.ts` and any context provider). Expose `permissions: string[]`.

- [ ] **Step 2: Gate nav entries**

In the navigation component, wrap each entry with `hasPermission(permissions, key)`:
- Workers nav → `Perm.workersRead`
- Tasks nav → `Perm.tasksRead`
- Sessions nav → `Perm.sessionsRead`
- Departments nav → `Perm.departmentsRead`
- Env → `Perm.envRead`
- Settings/System config → `Perm.systemConfigRead`
- Users nav → `Perm.usersManage`
- Roles nav → `Perm.rolesManage`

Example:

```tsx
import { Perm, hasPermission } from "../lib/permissions";
// ...
{hasPermission(permissions, Perm.usersManage) && (
  <NavLink to="/users">Users</NavLink>
)}
{hasPermission(permissions, Perm.rolesManage) && (
  <NavLink to="/roles">Roles</NavLink>
)}
```

- [ ] **Step 3: Hide write actions in existing pages (optional, where obvious)**

Where a page exposes a clearly-gated write button (e.g. "New worker" → `workersWrite`), wrap it with `hasPermission`. Backend already enforces; this is UX polish. Keep changes minimal and only where the permission mapping is unambiguous.

- [ ] **Step 4: Verify build + manual smoke**

Run: `cd web && npm run build`
Expected: success

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -m "feat(web): gate navigation and actions by permission"
```

---

## Phase E — Docs & verification

### Task 21: CHANGELOG + end-to-end verification

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add an English changelog entry**

Add under the current unreleased/next-version section in `CHANGELOG.md` (match the existing heading style):

```markdown
### Added
- Multi-user accounts with configurable role-based access control (RBAC). Each user can hold multiple roles; effective permissions are the union of their roles' permission keys.
- First-run setup wizard to create the initial super-admin. On upgrade, the web console shows the wizard until a super-admin exists.

### Changed
- Web login now authenticates against database users instead of the single `server.auth.username` / `server.auth.password` config credential. Those config fields are deprecated and ignored for login (JWT/token settings are unchanged).
```

> All changelog content must be in English (project rule).

- [ ] **Step 2: Full backend test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 3: Frontend build**

Run: `cd web && npm run build`
Expected: success

- [ ] **Step 4: Manual smoke test**

Run the server against a fresh temp DB, then:
1. Open the web UI → expect redirect to `/setup`.
2. Create super-admin → expect login + full nav.
3. Create a `member`-role user; log in as them → expect read-only nav, write calls 403.
4. Create a custom role, assign it, verify gating.
5. Attempt to delete the only super-admin → expect a 400 error surfaced in UI.

- [ ] **Step 5: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: changelog for multi-user RBAC"
```

---

## Self-Review Notes (for the planner)

- **Spec coverage:** tables/seed (Task 1), models (2), stores (3,4), bcrypt (6), catalog+resolver (7), JWT uid (8), middleware (9), login/me/password (10), setup wizard (11), users (12), roles+catalog (13), routes+guards (14), wiring+config deprecation (15), frontend helper/me (16), wizard (17), users page (18), roles page (19), nav gating (20), changelog (21). Every spec section maps to a task.
- **Type consistency:** `model.RoleID*` constants used in migration seed IDs and store/handler logic; `PermXxx` constants shared by catalog, routes, and frontend mirror (`web/src/lib/permissions.ts`); `UserWithRoles`/`RoleWithPermissions` consistent across store→handler→frontend types.
- **Super-admin safety enforced twice:** store blocks deleting `is_system` roles and ignores super-admin permission edits; handler blocks removing/disabling/deleting the last active super-admin.
- **Cache invalidation:** per-user on role/status change (Task 12), global on role-permission change (Task 13).
