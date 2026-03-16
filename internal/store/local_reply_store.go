package store

import (
	"context"
	"database/sql"
	"time"
)

// LocalReply is a row from the local_replies table.
type LocalReply struct {
	ID         string `json:"id"`
	SessionKey string `json:"session_key"`
	Content    string `json:"content"`
	CreatedAt  int64  `json:"created_at"`
}

// LocalReplyStore manages the local_replies table.
type LocalReplyStore struct {
	db *sql.DB
}

// NewLocalReplyStore constructs a LocalReplyStore.
func NewLocalReplyStore(db *sql.DB) *LocalReplyStore {
	return &LocalReplyStore{db: db}
}

// Create inserts a new reply row.
func (s *LocalReplyStore) Create(ctx context.Context, id, sessionKey, content string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO local_replies (id, session_key, content, created_at) VALUES (?, ?, ?, ?)`,
		id, sessionKey, content, time.Now().UnixMilli(),
	)
	return err
}

// ListBySession returns all replies for a session key ordered by created_at.
func (s *LocalReplyStore) ListBySession(ctx context.Context, sessionKey string) ([]LocalReply, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_key, content, created_at FROM local_replies
		 WHERE session_key = ? ORDER BY created_at ASC`,
		sessionKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	replies := []LocalReply{}
	for rows.Next() {
		var r LocalReply
		if err := rows.Scan(&r.ID, &r.SessionKey, &r.Content, &r.CreatedAt); err != nil {
			return nil, err
		}
		replies = append(replies, r)
	}
	return replies, rows.Err()
}

// DeleteBySession removes all replies for the given session key.
func (s *LocalReplyStore) DeleteBySession(ctx context.Context, sessionKey string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM local_replies WHERE session_key = ?`, sessionKey,
	)
	return err
}
