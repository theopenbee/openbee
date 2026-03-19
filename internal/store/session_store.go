package store

import (
	"context"
	"database/sql"
	"time"
)

// BeeAgentID is the agent_id value used for bee brain session tracking.
const BeeAgentID = "bee"

// SessionStore persists session context to the session_contexts table.
type SessionStore struct {
	db *sql.DB
}

// NewSessionStore constructs a SessionStore.
func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

// UpsertSessionContext writes or overwrites the session_id for (sessionKey, agentID).
func (s *SessionStore) UpsertSessionContext(ctx context.Context, sessionKey, agentID, sessionID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO bee_session_contexts (session_key, agent_id, session_id, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(session_key, agent_id) DO UPDATE
		 SET session_id = excluded.session_id, updated_at = excluded.updated_at`,
		sessionKey, agentID, sessionID, time.Now().UnixMilli(),
	)
	return err
}

// GetSessionContext returns the session_id for (sessionKey, agentID).
// Returns ("", nil) when no row exists — this is normal for the first message,
// not a database error. Returns non-nil error only on database failure.
func (s *SessionStore) GetSessionContext(ctx context.Context, sessionKey, agentID string) (string, error) {
	var sessionID string
	err := s.db.QueryRowContext(ctx,
		`SELECT session_id FROM bee_session_contexts WHERE session_key = ? AND agent_id = ?`,
		sessionKey, agentID,
	).Scan(&sessionID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return sessionID, err
}

// ClearSessionContexts deletes all session_contexts rows for sessionKey,
// resetting session state for bee and all workers under that key.
func (s *SessionStore) ClearSessionContexts(ctx context.Context, sessionKey string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM bee_session_contexts WHERE session_key = ?`,
		sessionKey,
	)
	return err
}
