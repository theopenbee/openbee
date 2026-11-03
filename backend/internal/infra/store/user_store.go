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

var ErrAlreadyInitialized = errors.New("system already initialized")

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

const userColumns = `id, username, password_hash, display_name, created_at, updated_at, password_changed_at`

// floorToSecond truncates a millisecond timestamp to second granularity so that
// password_changed_at lines up with the JWT `iat` claim, which is emitted with
// second precision. Tokens minted at or before this instant are then rejected in
// the auth middleware / refresh path, forcing a re-login after any password
// change.
func floorToSecond(ms int64) int64 { return ms / 1000 * 1000 }

func (s *UserStore) CreateFirst(username, plainPassword, displayName string) (model.User, error) {
	hash, err := auth.HashPassword(plainPassword)
	if err != nil {
		return model.User{}, err
	}
	now := time.Now().UnixMilli()
	u := model.User{
		ID:                uuid.New().String(),
		Username:          username,
		PasswordHash:      hash,
		DisplayName:       displayName,
		CreatedAt:         now,
		UpdatedAt:         now,
		PasswordChangedAt: floorToSecond(now),
	}
	res, err := s.db.Exec(
		`INSERT INTO bee_users (`+userColumns+`) SELECT ?, ?, ?, ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM bee_users)`,
		u.ID, u.Username, u.PasswordHash, u.DisplayName, u.CreatedAt, u.UpdatedAt, u.PasswordChangedAt,
	)
	if err != nil {
		return model.User{}, fmt.Errorf("insert user: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return model.User{}, err
	} else if n == 0 {
		return model.User{}, ErrAlreadyInitialized
	}
	return u, nil
}

func (s *UserStore) GetByID(id string) (model.User, error) {
	u, err := scanUser(s.db.QueryRow(`SELECT `+userColumns+` FROM bee_users WHERE id = ?`, id))
	if err != nil {
		return model.User{}, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

func (s *UserStore) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM bee_users`).Scan(&n)
	return n, err
}

func (s *UserStore) Authenticate(username, plainPassword string) (model.User, error) {
	u, err := scanUser(s.db.QueryRow(`SELECT `+userColumns+` FROM bee_users WHERE username = ?`, username))
	if err != nil {
		return model.User{}, ErrInvalidCredentials
	}
	if !auth.CheckPassword(u.PasswordHash, plainPassword) {
		return model.User{}, ErrInvalidCredentials
	}
	return u, nil
}

func (s *UserStore) SetPassword(id, plainPassword string) error {
	hash, err := auth.HashPassword(plainPassword)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	_, err = s.db.Exec(`UPDATE bee_users SET password_hash = ?, password_changed_at = ?, updated_at = ? WHERE id = ?`,
		hash, floorToSecond(now), now, id)
	return err
}

func scanUser(scanner interface{ Scan(...any) error }) (model.User, error) {
	var u model.User
	err := scanner.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.CreatedAt, &u.UpdatedAt, &u.PasswordChangedAt)
	return u, err
}

func (s *UserStore) PasswordChangedAt(userID string) (int64, error) {
	var passwordChangedAt int64
	err := s.db.QueryRow(`SELECT password_changed_at FROM bee_users WHERE id = ?`, userID).Scan(&passwordChangedAt)
	return passwordChangedAt, err
}

func (s *UserStore) AdminID() (string, bool, error) {
	var id string
	err := s.db.QueryRow(`SELECT id FROM bee_users ORDER BY created_at, id LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}
