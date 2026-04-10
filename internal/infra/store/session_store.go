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

// UpsertSessionContext writes or overwrites the session_id and engine for (sessionKey, agentID).
func (s *SessionStore) UpsertSessionContext(ctx context.Context, sessionKey, agentID, sessionID, engine string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO bee_session_contexts (session_key, agent_id, session_id, engine, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(session_key, agent_id) DO UPDATE
		 SET session_id = excluded.session_id, engine = excluded.engine, updated_at = excluded.updated_at`,
		sessionKey, agentID, sessionID, engine, time.Now().UnixMilli(),
	)
	return err
}

// GetSessionContext returns the session_id and engine for (sessionKey, agentID).
// Returns ("", "", nil) when no row exists — this is normal for the first message,
// not a database error. Returns non-nil error only on database failure.
func (s *SessionStore) GetSessionContext(ctx context.Context, sessionKey, agentID string) (sessionID, engine string, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT session_id, engine FROM bee_session_contexts WHERE session_key = ? AND agent_id = ?`,
		sessionKey, agentID,
	).Scan(&sessionID, &engine)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	return sessionID, engine, err
}

// GetSessionContextForEngine returns the session_id for (sessionKey, agentID) only when the
// stored engine matches the requested engine. Rows with an empty engine (legacy data recorded
// before engine tracking was added) are also returned to preserve backward compatibility.
// Returns ("", nil) when no matching row exists.
func (s *SessionStore) GetSessionContextForEngine(ctx context.Context, sessionKey, agentID, engine string) (sessionID string, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT session_id FROM bee_session_contexts WHERE session_key = ? AND agent_id = ? AND (engine = ? OR engine = '')`,
		sessionKey, agentID, engine,
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

// SessionAgent represents one agent's session context entry, enriched with
// a human-readable name.
type SessionAgent struct {
	AgentID   string
	AgentType string // "bee" or "worker"
	Name      string // worker name, "bee", or "(deleted)"
	UpdatedAt int64
}

// ListSessionContexts returns all agents with session contexts for sessionKey,
// ordered by updated_at DESC. Worker names are resolved via LEFT JOIN; deleted
// workers appear as "(deleted)". AgentType is derived in Go from AgentID.
func (s *SessionStore) ListSessionContexts(ctx context.Context, sessionKey string) ([]SessionAgent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sc.agent_id, sc.updated_at,
		       COALESCE(w.name, CASE WHEN sc.agent_id = 'bee' THEN 'bee' ELSE '(deleted)' END) AS name
		FROM bee_session_contexts sc
		LEFT JOIN bee_workers w ON w.id = sc.agent_id
		WHERE sc.session_key = ?
		ORDER BY sc.updated_at DESC`,
		sessionKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SessionAgent
	for rows.Next() {
		var a SessionAgent
		if err := rows.Scan(&a.AgentID, &a.UpdatedAt, &a.Name); err != nil {
			return nil, err
		}
		if a.AgentID == BeeAgentID {
			a.AgentType = "bee"
		} else {
			a.AgentType = "worker"
		}
		result = append(result, a)
	}
	if result == nil {
		result = []SessionAgent{}
	}
	return result, rows.Err()
}

// DeleteWorkerSessionContext removes the session context row for one worker.
// Deleting a non-existent row is not an error.
func (s *SessionStore) DeleteWorkerSessionContext(ctx context.Context, sessionKey, workerID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM bee_session_contexts WHERE session_key = ? AND agent_id = ?`,
		sessionKey, workerID,
	)
	return err
}
