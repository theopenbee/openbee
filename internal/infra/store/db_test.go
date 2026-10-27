package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestInitDB_AppliesPragmas(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer db.Close()

	var mode string
	require.NoError(t, db.QueryRow("PRAGMA journal_mode").Scan(&mode))
	assert.Equal(t, "wal", mode)
	var timeout int
	require.NoError(t, db.QueryRow("PRAGMA busy_timeout").Scan(&timeout))
	assert.Equal(t, 5000, timeout)
}

func TestInitDB_WaitsForAnotherWriter(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := InitDB(dbPath)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE lock_probe (v TEXT)`)
	require.NoError(t, err)

	other, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer other.Close()
	tx, err := other.Begin()
	require.NoError(t, err)
	_, err = tx.Exec(`INSERT INTO lock_probe (v) VALUES ('held')`)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		time.Sleep(300 * time.Millisecond)
		done <- tx.Commit()
	}()

	_, err = db.Exec(`INSERT INTO lock_probe (v) VALUES ('waiting')`)
	require.NoError(t, err)
	require.NoError(t, <-done)
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
		message_id   TEXT NOT NULL DEFAULT '',
		execution_id TEXT NOT NULL DEFAULT '',
		worker_id    TEXT NOT NULL DEFAULT '',
		type         TEXT NOT NULL DEFAULT 'immediate',
		status       TEXT NOT NULL DEFAULT 'pending'
	)`)
	require.NoError(t, err)
	// bee_outbound_messages is required by migration 34 (index on sent_at).
	_, err = db.Exec(`CREATE TABLE bee_outbound_messages (
		id          TEXT PRIMARY KEY,
		sent_at     INTEGER NOT NULL,
		platform    TEXT NOT NULL DEFAULT '',
		content     TEXT NOT NULL DEFAULT '',
		source_type TEXT NOT NULL DEFAULT '',
		source_id   TEXT NOT NULL DEFAULT ''
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

func TestMigration_BackfillsTaskSessionTarget(t *testing.T) {
	db := openPreMigrationDB(t, 51)

	for _, q := range []string{
		seedWorkerSQL("w1"),
		seedMessageSQL("m1", "beta:chat_1:user_1"),
		`INSERT INTO bee_tasks (id, message_id, worker_id, instruction, type, created_at, updated_at)
		 VALUES ('t-linked','m1','w1','go','scheduled',1,1), ('t-orphan','missing','w1','go','scheduled',1,1)`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}

	require.NoError(t, migrate(db))

	var sessionKey, platform string
	require.NoError(t, db.QueryRow(`SELECT session_key, platform FROM bee_tasks WHERE id = 't-linked'`).Scan(&sessionKey, &platform))
	assert.Equal(t, "beta:chat_1:user_1", sessionKey)
	assert.Equal(t, "beta", platform)

	require.NoError(t, db.QueryRow(`SELECT session_key, platform FROM bee_tasks WHERE id = 't-orphan'`).Scan(&sessionKey, &platform))
	assert.Empty(t, sessionKey)
	assert.Empty(t, platform)
}

func openPreMigrationDB(t *testing.T, version int) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", t.TempDir()+"/pre.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE bee_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	)`)
	require.NoError(t, err)

	var before []migration
	for _, m := range migrations {
		if m.version < version {
			before = append(before, m)
		}
	}
	require.NoError(t, applyMigrations(db, before))
	return db
}

func seedLegacyUser(t *testing.T, db *sql.DB, id, status string, createdAt int64, roleIDs ...string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO bee_users (id, username, password_hash, status, created_at, updated_at) VALUES (?, ?, 'x', ?, ?, ?)`,
		id, id, status, createdAt, createdAt)
	require.NoError(t, err)
	for _, rid := range roleIDs {
		_, err := db.Exec(`INSERT INTO bee_user_roles (user_id, role_id, created_at) VALUES (?, ?, 1)`, id, rid)
		require.NoError(t, err)
	}
}

func remainingUserIDs(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT id FROM bee_users ORDER BY id`)
	require.NoError(t, err)
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}

func TestMigration_DropsRBACAndKeepsFirstActiveSuperAdmin(t *testing.T) {
	db := openPreMigrationDB(t, 53)
	_, err := db.Exec(`INSERT INTO bee_roles (id, name, created_at, updated_at) VALUES ('r-ops', 'ops', 1, 1)`)
	require.NoError(t, err)
	seedLegacyUser(t, db, "operator", "active", 1, "r-ops")
	seedLegacyUser(t, db, "disabled-admin", "disabled", 2, "sysrole_superadmin")
	seedLegacyUser(t, db, "first-admin", "active", 3, "sysrole_superadmin")
	seedLegacyUser(t, db, "later-admin", "active", 4, "sysrole_superadmin")

	require.NoError(t, migrate(db))

	assert.Equal(t, []string{"first-admin"}, remainingUserIDs(t, db))
	for _, table := range []string{"bee_roles", "bee_role_permissions", "bee_user_roles"} {
		var n int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n))
		assert.Zero(t, n, table)
	}
	for _, column := range []string{"status", "created_by"} {
		var n int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('bee_users') WHERE name=?`, column).Scan(&n))
		assert.Zero(t, n, column)
	}
}

func TestMigration_DropRBACFallsBackToEarliestActiveUser(t *testing.T) {
	db := openPreMigrationDB(t, 53)
	seedLegacyUser(t, db, "disabled", "disabled", 1)
	seedLegacyUser(t, db, "active", "active", 2)
	seedLegacyUser(t, db, "later", "active", 3)

	require.NoError(t, migrate(db))

	assert.Equal(t, []string{"active"}, remainingUserIDs(t, db))
}

func TestMigration_DropRBACOnEmptyUsers(t *testing.T) {
	db := openPreMigrationDB(t, 53)

	require.NoError(t, migrate(db))

	assert.Empty(t, remainingUserIDs(t, db))
}

func sessionContextKeys(t *testing.T, db *sql.DB, agentID string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT session_key FROM bee_session_contexts WHERE agent_id = ? ORDER BY session_key`, agentID)
	require.NoError(t, err)
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		require.NoError(t, rows.Scan(&key))
		keys = append(keys, key)
	}
	require.NoError(t, rows.Err())
	return keys
}

func TestMigrationSplitsLocalWorkerSessions(t *testing.T) {
	db := openPreMigrationDB(t, 57)
	for _, q := range []string{
		seedWorkerSQL("w1"),
		`INSERT INTO bee_session_contexts (session_key, agent_id, engine, session_id, updated_at) VALUES
			('local:u1','bee','claude','s-bee',1),
			('local:u1','w1','claude','s-w1',1),
			('telegram:1:2','w1','claude','s-tg',1),
			('local:default','w1','claude','s-default',1),
			('local:u1','w3','claude','s-w3-old',1),
			('local:u1:worker:w3','w3','claude','s-w3-new',1)`,
		seedMessageSQL("m1", "local:u1"),
		`INSERT INTO bee_tasks (id, message_id, worker_id, instruction, type, status, session_key, platform, created_at, updated_at) VALUES
			('t-sched','m1','w1','go','scheduled','pending','local:u1','local',1,1),
			('t-count','m1','w1','go','countdown','pending','local:u1','local',1,1),
			('t-imm','m1','w1','go','immediate','pending','local:u1','local',1,1),
			('t-done','m1','w1','go','countdown','completed','local:u1','local',1,1),
			('t-running','m1','w1','go','scheduled','running','local:u1','local',1,1),
			('t-tg','m1','w1','go','scheduled','pending','telegram:1:2','telegram',1,1),
			('t-default','m1','w1','go','scheduled','pending','local:default','local',1,1),
			('t-split','m1','w1','go','scheduled','pending','local:u1:worker:w1','local',1,1)`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}

	require.NoError(t, migrate(db))

	assert.Equal(t, []string{"local:u1"}, sessionContextKeys(t, db, "bee"))
	assert.Equal(t, []string{"local:default", "local:u1:worker:w1", "telegram:1:2"}, sessionContextKeys(t, db, "w1"))
	assert.Equal(t, []string{"local:u1", "local:u1:worker:w3"}, sessionContextKeys(t, db, "w3"))

	var sessionID string
	require.NoError(t, db.QueryRow(`SELECT session_id FROM bee_session_contexts WHERE session_key = 'local:u1:worker:w1' AND agent_id = 'w1'`).Scan(&sessionID))
	assert.Equal(t, "s-w1", sessionID)
	require.NoError(t, db.QueryRow(`SELECT session_id FROM bee_session_contexts WHERE session_key = 'local:u1:worker:w3' AND agent_id = 'w3'`).Scan(&sessionID))
	assert.Equal(t, "s-w3-new", sessionID)

	for id, want := range map[string]string{
		"t-sched":   "local:u1:worker:w1",
		"t-count":   "local:u1:worker:w1",
		"t-imm":     "local:u1",
		"t-done":    "local:u1",
		"t-running": "local:u1",
		"t-tg":      "telegram:1:2",
		"t-default": "local:default",
		"t-split":   "local:u1:worker:w1",
	} {
		var got string
		require.NoError(t, db.QueryRow(`SELECT session_key FROM bee_tasks WHERE id = ?`, id).Scan(&got))
		assert.Equal(t, want, got, id)
	}
}

func TestMigrationStripsWorkerPrefixOnlyOnExactName(t *testing.T) {
	db := openPreMigrationDB(t, 57)
	for _, q := range []string{
		`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','林知夏','/','idle',1,1)`,
		`INSERT INTO bee_outbound_messages (id, session_key, platform, content, source_type, source_id, sent_at, created_at) VALUES
			('o-exact','local:u1','local','林知夏' || char(10) || 'hello','worker','w1',1,1),
			('o-short','local:u1','local','林知' || char(10) || 'hello','worker','w1',1,1),
			('o-tg','telegram:1:2','telegram','林知夏' || char(10) || 'hello','worker','w1',1,1),
			('o-bee','local:u1','local','林知夏' || char(10) || 'hello','bee','',1,1),
			('o-unknown','local:u1','local','林知夏' || char(10) || 'hello','worker','missing',1,1),
			('o-name-only','local:u1','local','林知夏','worker','w1',1,1)`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}

	require.NoError(t, migrate(db))

	for id, want := range map[string]string{
		"o-exact":     "hello",
		"o-short":     "林知\nhello",
		"o-tg":        "林知夏\nhello",
		"o-bee":       "林知夏\nhello",
		"o-unknown":   "林知夏\nhello",
		"o-name-only": "林知夏",
	} {
		var got string
		require.NoError(t, db.QueryRow(`SELECT content FROM bee_outbound_messages WHERE id = ?`, id).Scan(&got))
		assert.Equal(t, want, got, id)
	}
}

func TestMigrationAddsLocalChatColumns(t *testing.T) {
	db := newTestDB(t, seedWorkerSQL("w1"), seedMessageSQL("m1", "local:u1"))

	var senderType string
	require.NoError(t, db.QueryRow(`SELECT sender_type FROM bee_platform_messages WHERE id = 'm1'`).Scan(&senderType))
	assert.Equal(t, "user", senderType)

	_, err := db.Exec(`INSERT INTO bee_outbound_messages (id, session_key, platform, sent_at, created_at) VALUES ('o1','local:u1','local',1,1)`)
	require.NoError(t, err)
	var kind, payload string
	require.NoError(t, db.QueryRow(`SELECT kind, payload FROM bee_outbound_messages WHERE id = 'o1'`).Scan(&kind, &payload))
	assert.Equal(t, "message", kind)
	assert.Empty(t, payload)

	_, err = db.Exec(`INSERT INTO bee_local_reads (user_id, conversation_id, last_read_at) VALUES ('u1','bee',5)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO bee_local_reads (user_id, conversation_id, last_read_at) VALUES ('u1','bee',6)`)
	require.Error(t, err)
}
