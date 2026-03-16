package store

import (
	"context"
	"database/sql"
	"time"
)

// LocalSession is a row from the local_sessions table.
type LocalSession struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// LocalSessionStore manages the local_sessions table.
type LocalSessionStore struct {
	db *sql.DB
}

// NewLocalSessionStore constructs a LocalSessionStore.
func NewLocalSessionStore(db *sql.DB) *LocalSessionStore {
	return &LocalSessionStore{db: db}
}

// Create inserts a new local session.
func (s *LocalSessionStore) Create(ctx context.Context, id, name string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO local_sessions (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		id, name, now, now,
	)
	return err
}

// List returns all sessions ordered by updated_at descending.
func (s *LocalSessionStore) List(ctx context.Context) ([]LocalSession, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, created_at, updated_at FROM local_sessions ORDER BY updated_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := []LocalSession{}
	for rows.Next() {
		var sess LocalSession
		if err := rows.Scan(&sess.ID, &sess.Name, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

// Delete removes a session by ID.
func (s *LocalSessionStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM local_sessions WHERE id = ?`, id)
	return err
}

// TouchUpdatedAt bumps updated_at to now for the given session.
// Called by the HTTP handler on every message send so the session list stays sorted by activity.
func (s *LocalSessionStore) TouchUpdatedAt(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE local_sessions SET updated_at = ? WHERE id = ?`,
		time.Now().UnixMilli(), id,
	)
	return err
}
