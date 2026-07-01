package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/apperr"
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
	// Drain the result set before issuing nested queries: the DB pool is capped
	// at a single connection (SetMaxOpenConns(1)), so holding this iterator open
	// while querying permissions would deadlock.
	var roles []model.Role
	for rows.Next() {
		r, err := scanRole(rows)
		if err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	permsByRole, err := s.permissionsByRole()
	if err != nil {
		return nil, err
	}
	out := make([]model.RoleWithPermissions, 0, len(roles))
	for _, r := range roles {
		perms := permsByRole[r.ID]
		if perms == nil {
			perms = []string{}
		}
		out = append(out, model.RoleWithPermissions{Role: r, Permissions: perms})
	}
	return out, nil
}

func (s *RoleStore) Update(r model.Role, permissions []string) error {
	var isSystem int
	if err := s.db.QueryRow(`SELECT is_system FROM bee_roles WHERE id = ?`, r.ID).Scan(&isSystem); err != nil {
		return fmt.Errorf("get role: %w", err)
	}
	if isSystem == 1 {
		// System roles are locked: their name, description, and permissions are
		// all immutable. Ignore any incoming changes.
		return nil
	}

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
		return apperr.New("system_role_undeletable", "system roles cannot be deleted")
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

// permissionsByRole loads every role's permissions in a single query, bucketed
// by role id. List uses this instead of one permissionsFor call per role.
func (s *RoleStore) permissionsByRole() (map[string][]string, error) {
	rows, err := s.db.Query(`SELECT role_id, permission FROM bee_role_permissions ORDER BY role_id, permission`)
	if err != nil {
		return nil, fmt.Errorf("get role permissions: %w", err)
	}
	defer rows.Close()
	byRole := map[string][]string{}
	for rows.Next() {
		var roleID, p string
		if err := rows.Scan(&roleID, &p); err != nil {
			return nil, err
		}
		byRole[roleID] = append(byRole[roleID], p)
	}
	return byRole, rows.Err()
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
