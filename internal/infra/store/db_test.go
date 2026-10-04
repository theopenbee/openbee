package store

import (
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestInitDB(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	db, err := InitDB(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Verify tables exist
	tables := []string{"bee_workers", "bee_executions"}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		assert.NoError(t, err, "table %s not found", table)
	}

	_ = os.Remove(dbPath)
}

func TestInitDB_PlatformMessagesTable(t *testing.T) {
	db := newTestDB(t)

	_, err := db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, received_at, created_at, updated_at) VALUES ('x','sk','p','c',1,1,1)`)
	require.NoError(t, err)
}

func TestMigrations_Idempotent(t *testing.T) {
	db := newTestDB(t)

	// Run migrate a second time — must not error
	require.NoError(t, migrate(db))

	// Each migration version should appear exactly once
	for _, m := range migrations {
		var count int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM bee_migrations WHERE version = ?`, m.version).Scan(&count))
		assert.Equal(t, 1, count, "migration version %d: want 1 row", m.version)
	}
}

func TestMigrations_TableExists(t *testing.T) {
	db := newTestDB(t)

	var name string
	require.NoError(t, db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='bee_migrations'`).Scan(&name))

	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM bee_migrations`).Scan(&count))
	assert.Equal(t, len(migrations), count)
}

func TestInitDB_TasksTable(t *testing.T) {
	db := newTestDB(t, seedWorkerSQL("w1"), seedMessageSQL("m1", "sk"))

	_, err := db.Exec(`INSERT INTO bee_tasks
		(id, message_id, worker_id, instruction, type, created_at, updated_at)
		VALUES ('t1','m1','w1','do it','immediate',1,1)`)
	require.NoError(t, err)
}

func TestMigrations_SkipsApplied(t *testing.T) {
	db := newTestDB(t)

	// Confirm version 1 was applied exactly once
	var countBefore int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM bee_migrations WHERE version = 1`).Scan(&countBefore))
	require.Equal(t, 1, countBefore)

	// Re-run migrate — version 1 should be skipped
	require.NoError(t, migrate(db))

	var countAfter int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM bee_migrations WHERE version = 1`).Scan(&countAfter))
	assert.Equal(t, 1, countAfter, "version 1 should appear exactly once after re-run")
}

func TestMigration_UpgradesSessionContextsToPerEngineSchema(t *testing.T) {
	dbPath := t.TempDir() + "/legacy.db"
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE bee_migrations (
		version INTEGER PRIMARY KEY,
		name    TEXT NOT NULL
	)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE bee_session_contexts (
		session_key TEXT NOT NULL,
		agent_id    TEXT NOT NULL,
		session_id  TEXT NOT NULL,
		updated_at  INTEGER NOT NULL,
		PRIMARY KEY (session_key, agent_id)
	)`)
	require.NoError(t, err)
	// bee_workers is required by migrations that run after migration 29.
	_, err = db.Exec(`CREATE TABLE bee_workers (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		memory      TEXT NOT NULL DEFAULT '',
		work_dir    TEXT NOT NULL DEFAULT '',
		status      TEXT NOT NULL DEFAULT 'idle',
		created_at  INTEGER NOT NULL,
		updated_at  INTEGER NOT NULL
	)`)
	require.NoError(t, err)
	// bee_executions is required by migration 32 (index on started_at).
	_, err = db.Exec(`CREATE TABLE bee_executions (
		id          TEXT PRIMARY KEY,
		worker_id   TEXT NOT NULL,
		session_id  TEXT NOT NULL,
		started_at  INTEGER NOT NULL,
		completed_at INTEGER,
		status      TEXT NOT NULL DEFAULT '',
		result      TEXT NOT NULL DEFAULT ''
	)`)
	require.NoError(t, err)
	// bee_tasks is required by migration 45 (backfill task_id into bee_executions).
	_, err = db.Exec(`CREATE TABLE bee_tasks (
		id           TEXT PRIMARY KEY,
		execution_id TEXT NOT NULL DEFAULT ''
	)`)
	require.NoError(t, err)
	// bee_outbound_messages is required by migration 34 (index on sent_at).
	_, err = db.Exec(`CREATE TABLE bee_outbound_messages (
		id      TEXT PRIMARY KEY,
		sent_at INTEGER NOT NULL
	)`)
	require.NoError(t, err)
	// bee_platform_messages is required by migration 31 (drop retry_count).
	_, err = db.Exec(`CREATE TABLE bee_platform_messages (
		id              TEXT PRIMARY KEY,
		session_key     TEXT NOT NULL,
		platform        TEXT NOT NULL,
		content         TEXT NOT NULL,
		status          TEXT NOT NULL DEFAULT 'received',
		merged_into     TEXT NOT NULL DEFAULT '',
		platform_msg_id TEXT NOT NULL DEFAULT '',
		raw             TEXT NOT NULL DEFAULT '',
		received_at     INTEGER NOT NULL,
		created_at      INTEGER NOT NULL,
		updated_at      INTEGER NOT NULL,
		retry_count     INTEGER NOT NULL DEFAULT 0
	)`)
	require.NoError(t, err)
	// bee_memories is required by migration 40 (rename to bee_constraints).
	_, err = db.Exec(`CREATE TABLE bee_memories (
		id         TEXT PRIMARY KEY,
		scope      TEXT NOT NULL,
		key        TEXT NOT NULL,
		value      TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		UNIQUE(scope, key)
	)`)
	require.NoError(t, err)
	// bee_departments / bee_worker_departments are required by migration 50 (purge orphaned link rows).
	_, err = db.Exec(`CREATE TABLE bee_departments (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL,
		parent_id  TEXT,
		sort_order INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE bee_worker_departments (
		worker_id     TEXT NOT NULL REFERENCES bee_workers(id),
		department_id TEXT NOT NULL REFERENCES bee_departments(id),
		created_at    INTEGER NOT NULL,
		PRIMARY KEY (worker_id, department_id)
	)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO bee_session_contexts (session_key, agent_id, session_id, updated_at)
		VALUES ('sk', 'bee', 'legacy-sid', 1)`)
	require.NoError(t, err)
	for _, m := range migrations {
		if m.version >= 29 {
			break
		}
		_, err := db.Exec(`INSERT INTO bee_migrations (version, name) VALUES (?, ?)`, m.version, m.name)
		require.NoError(t, err)
	}

	require.NoError(t, migrate(db))

	var sessionID, engine string
	require.NoError(t, db.QueryRow(`SELECT session_id, engine
		FROM bee_session_contexts
		WHERE session_key = 'sk' AND agent_id = 'bee' AND engine = ?`, ai.EngineClaude).Scan(&sessionID, &engine))
	require.Equal(t, "legacy-sid", sessionID)
	require.Equal(t, ai.EngineClaude, engine)

	_, err = db.Exec(`INSERT INTO bee_session_contexts (session_key, agent_id, session_id, updated_at, engine)
		VALUES ('sk', 'bee', 'codex-sid', 2, 'codex')`)
	require.NoError(t, err)

	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM bee_session_contexts WHERE session_key = 'sk' AND agent_id = 'bee'`).Scan(&count))
	require.Equal(t, 2, count, "expected 2 per-engine rows after migration")
}
