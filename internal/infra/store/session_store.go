package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/theopenbee/openbee/internal/bridge"
)

// BeeAgentID is the agent_id value used for bee brain session tracking.
const BeeAgentID = "bee"

// BeeAgentType is the AgentType value for the bee brain in session contexts.
const BeeAgentType = "bee"

// WorkerAgentType is the AgentType value for worker agents in session contexts.
const WorkerAgentType = "worker"

const defaultSessionEngine = bridge.EngineClaude

// SessionStore persists session context to the session_contexts table.
type SessionStore struct {
	db *sql.DB
}

// NewSessionStore constructs a SessionStore.
func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

func normalizeSessionEngine(engine string) string {
	if engine == "" {
		return defaultSessionEngine
	}
	return engine
}

// UpsertSessionContext writes or overwrites the session_id for one
// (sessionKey, agentID, engine) tuple.
func (s *SessionStore) UpsertSessionContext(ctx context.Context, sessionKey, agentID, sessionID, engine string) error {
	engine = normalizeSessionEngine(engine)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO bee_session_contexts (session_key, agent_id, session_id, engine, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(session_key, agent_id, engine) DO UPDATE
		 SET session_id = excluded.session_id, updated_at = excluded.updated_at`,
		sessionKey, agentID, sessionID, engine, time.Now().UnixMilli(),
	)
	return err
}

// GetSessionContext returns the latest session_id and engine across all engines
// for (sessionKey, agentID). Returns ("", "", nil) when no row exists.
func (s *SessionStore) GetSessionContext(ctx context.Context, sessionKey, agentID string) (sessionID, engine string, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT session_id, engine
		 FROM bee_session_contexts
		 WHERE session_key = ? AND agent_id = ?
		 ORDER BY updated_at DESC
		 LIMIT 1`,
		sessionKey, agentID,
	).Scan(&sessionID, &engine)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	return sessionID, engine, err
}

// GetSessionContextForEngine returns the session_id for (sessionKey, agentID) only when the
// stored engine matches the requested engine.
// Returns ("", nil) when no matching row exists.
func (s *SessionStore) GetSessionContextForEngine(ctx context.Context, sessionKey, agentID, engine string) (sessionID string, err error) {
	engine = normalizeSessionEngine(engine)
	err = s.db.QueryRowContext(ctx,
		`SELECT session_id FROM bee_session_contexts WHERE session_key = ? AND agent_id = ? AND engine = ?`,
		sessionKey, agentID, engine,
	).Scan(&sessionID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return sessionID, err
}

// DeleteSessionContextForEngine removes one session context row for
// (sessionKey, agentID, engine). Returns (true, nil) if a row was deleted,
// (false, nil) if the row did not exist, or (false, err) on DB error.
func (s *SessionStore) DeleteSessionContextForEngine(ctx context.Context, sessionKey, agentID, engine string) (bool, error) {
	engine = normalizeSessionEngine(engine)
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM bee_session_contexts WHERE session_key = ? AND agent_id = ? AND engine = ?`,
		sessionKey, agentID, engine,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// activeWorkerSessionContextsFilter matches session context rows for non-bee agents
// whose engine is currently "active":
//   - worker has explicit engine → row matches that engine
//   - worker has no engine set → row matches the bee default (first ? param)
//   - worker deleted (no row in bee_workers) → all their rows are considered orphans
//
// Callers must bind the bee default engine exactly once to the ? placeholder.
const activeWorkerSessionContextsFilter = `
	agent_id != 'bee'
	AND (
	      EXISTS (SELECT 1 FROM bee_workers w
	              WHERE w.id = bee_session_contexts.agent_id AND w.engine != ''
	                AND w.engine = bee_session_contexts.engine)
	   OR EXISTS (SELECT 1 FROM bee_workers w
	              WHERE w.id = bee_session_contexts.agent_id AND w.engine = ''
	                AND bee_session_contexts.engine = ?)
	   OR NOT EXISTS (SELECT 1 FROM bee_workers w WHERE w.id = bee_session_contexts.agent_id)
	)`

// ClearSessionContexts deletes session context rows for sessionKey, scoped to
// each agent's currently active engine:
//   - bee: only the specified beeEngine row is removed.
//   - workers: only the row matching their configured engine (bee_workers.engine)
//     is removed; workers with no engine set fall back to beeEngine.
//   - deleted workers (absent from bee_workers): all their rows are removed as
//     orphaned data.
func (s *SessionStore) ClearSessionContexts(ctx context.Context, sessionKey, beeEngine string) error {
	beeEngine = normalizeSessionEngine(beeEngine)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM bee_session_contexts
		 WHERE session_key = ? AND agent_id = 'bee' AND engine = ?`,
		sessionKey, beeEngine,
	); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM bee_session_contexts
		 WHERE session_key = ? AND `+activeWorkerSessionContextsFilter,
		sessionKey, beeEngine,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// SessionAgent represents one agent's session context entry, enriched with
// a human-readable name.
type SessionAgent struct {
	AgentID   string
	AgentType string // "bee" or "worker"
	Engine    string
	Name      string // worker name, "bee", or "(deleted)"
	UpdatedAt int64
}

// scanSessionAgents reads all rows into a SessionAgent slice, deriving AgentType from AgentID.
func scanSessionAgents(rows *sql.Rows) ([]SessionAgent, error) {
	var result []SessionAgent
	for rows.Next() {
		var a SessionAgent
		if err := rows.Scan(&a.AgentID, &a.Engine, &a.UpdatedAt, &a.Name); err != nil {
			return nil, err
		}
		if a.AgentID == BeeAgentID {
			a.AgentType = BeeAgentType
		} else {
			a.AgentType = WorkerAgentType
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// ListSessionContexts returns all agent/engine session contexts for sessionKey,
// ordered by updated_at DESC. Worker names are resolved via LEFT JOIN; deleted
// workers appear as "(deleted)". AgentType is derived in Go from AgentID.
func (s *SessionStore) ListSessionContexts(ctx context.Context, sessionKey string) ([]SessionAgent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sc.agent_id, sc.engine, sc.updated_at,
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
	return scanSessionAgents(rows)
}

// ListActiveSessionContexts returns only the session contexts that would be
// cleared by ClearSessionContexts for the given beeEngine.
func (s *SessionStore) ListActiveSessionContexts(ctx context.Context, sessionKey, beeEngine string) ([]SessionAgent, error) {
	beeEngine = normalizeSessionEngine(beeEngine)
	rows, err := s.db.QueryContext(ctx, `
		SELECT bee_session_contexts.agent_id, bee_session_contexts.engine, bee_session_contexts.updated_at,
		       COALESCE(w.name, CASE WHEN bee_session_contexts.agent_id = 'bee' THEN 'bee' ELSE '(deleted)' END) AS name
		FROM bee_session_contexts
		LEFT JOIN bee_workers w ON w.id = bee_session_contexts.agent_id
		WHERE session_key = ?
		  AND (
		        (bee_session_contexts.agent_id = 'bee' AND bee_session_contexts.engine = ?)
		     OR (`+activeWorkerSessionContextsFilter+`)
		      )
		ORDER BY bee_session_contexts.updated_at DESC`,
		sessionKey, beeEngine, beeEngine,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessionAgents(rows)
}

// DeleteWorkerSessionContext removes all session context rows for one worker
// under a sessionKey across engines.
// Deleting a non-existent row is not an error.
func (s *SessionStore) DeleteWorkerSessionContext(ctx context.Context, sessionKey, workerID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM bee_session_contexts WHERE session_key = ? AND agent_id = ?`,
		sessionKey, workerID,
	)
	return err
}
