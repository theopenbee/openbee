package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/apperr"
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

const userColumns = `id, username, password_hash, display_name, status, created_by, created_at, updated_at, password_changed_at`

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
	// Floored to the second to match the JWT `iat` granularity, same as SetPassword.
	u.PasswordChangedAt = now / 1000 * 1000

	tx, err := s.db.Begin()
	if err != nil {
		return model.UserWithRoles{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(
		`INSERT INTO bee_users (`+userColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.PasswordHash, u.DisplayName, u.Status, u.CreatedBy, u.CreatedAt, u.UpdatedAt, u.PasswordChangedAt,
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
	// Drain the result set before issuing nested queries: the DB pool is capped
	// at a single connection (SetMaxOpenConns(1)), so holding this iterator open
	// while querying roles would deadlock.
	var users []model.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rolesByUser, err := s.rolesByUser()
	if err != nil {
		return nil, err
	}
	out := make([]model.UserWithRoles, 0, len(users))
	for _, u := range users {
		roles := rolesByUser[u.ID]
		if roles == nil {
			roles = []model.Role{}
		}
		out = append(out, model.UserWithRoles{User: u, Roles: roles})
	}
	return out, nil
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

// UpdateProfile changes a user's login username and display name. It returns a
// coded username_taken error when the new username collides with another user
// (the bee_users.username UNIQUE constraint), or user_not_found when no row
// matches id.
func (s *UserStore) UpdateProfile(id, username, displayName string) error {
	res, err := s.db.Exec(`UPDATE bee_users SET username = ?, display_name = ?, updated_at = ? WHERE id = ?`,
		username, displayName, time.Now().UnixMilli(), id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return apperr.New("username_taken", "username already taken")
		}
		return fmt.Errorf("update user profile: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperr.New("user_not_found", "user not found")
	}
	return nil
}

func (s *UserStore) SetPassword(id, plainPassword string) error {
	hash, err := auth.HashPassword(plainPassword)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	// Floor to the second so it lines up with the JWT `iat` claim (second
	// granularity). Tokens issued at or before this instant are then rejected in
	// the auth middleware / refresh path, forcing a re-login on every session.
	// This is the single write choke point for both self-service change and the
	// admin ResetPassword flow, so both invalidate existing sessions.
	passwordChangedAt := now / 1000 * 1000
	_, err = s.db.Exec(`UPDATE bee_users SET password_hash = ?, password_changed_at = ?, updated_at = ? WHERE id = ?`,
		hash, passwordChangedAt, now, id)
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

// rolesByUser loads every user's roles in a single query, bucketed by user id.
// List uses this instead of one rolesFor call per user.
func (s *UserStore) rolesByUser() (map[string][]model.Role, error) {
	rows, err := s.db.Query(`
		SELECT ur.user_id, r.id, r.name, r.description, r.is_system, r.created_at, r.updated_at
		FROM bee_user_roles ur
		JOIN bee_roles r ON r.id = ur.role_id
		ORDER BY r.is_system DESC, r.name`)
	if err != nil {
		return nil, fmt.Errorf("roles for users: %w", err)
	}
	defer rows.Close()
	byUser := map[string][]model.Role{}
	for rows.Next() {
		var userID string
		var r model.Role
		var isSystem int
		if err := rows.Scan(
			&userID, &r.ID, &r.Name, &r.Description, &isSystem, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		r.IsSystem = isSystem == 1
		byUser[userID] = append(byUser[userID], r)
	}
	return byUser, rows.Err()
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
	err := scanner.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Status, &u.CreatedBy, &u.CreatedAt, &u.UpdatedAt, &u.PasswordChangedAt)
	return u, err
}

// UserAuthState returns a user's account status and last password-change
// timestamp in one query (implements auth.UserStatusLoader). The middleware
// uses both to reject disabled accounts and tokens issued before a password
// change, without a second round-trip.
func (s *UserStore) UserAuthState(userID string) (string, int64, error) {
	var status string
	var passwordChangedAt int64
	err := s.db.QueryRow(`SELECT status, password_changed_at FROM bee_users WHERE id = ?`, userID).
		Scan(&status, &passwordChangedAt)
	return status, passwordChangedAt, err
}

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
