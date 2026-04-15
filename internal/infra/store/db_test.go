package store

import (
	"database/sql"
	"os"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestInitDB(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// Verify tables exist
	tables := []string{"bee_workers", "bee_executions"}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", table, err)
		}
	}

	_ = os.Remove(dbPath)
}

func TestInitDB_PlatformMessagesTable(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, received_at, created_at, updated_at) VALUES ('x','sk','p','c',1,1,1)`)
	if err != nil {
		t.Fatalf("platform_messages table not created: %v", err)
	}
}

func TestMigrations_Idempotent(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	// Run migrate a second time — must not error
	if err := migrate(db); err != nil {
		t.Fatalf("second migrate() call failed: %v", err)
	}

	// Each migration version should appear exactly once
	for _, m := range migrations {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM bee_migrations WHERE version = ?`, m.version).Scan(&count); err != nil {
			t.Fatalf("querying bee_migrations for version %d: %v", m.version, err)
		}
		if count != 1 {
			t.Errorf("migration version %d: want 1 row, got %d", m.version, count)
		}
	}
}

func TestMigrations_TableExists(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	var name string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='bee_migrations'`).Scan(&name); err != nil {
		t.Fatalf("bee_migrations table not found: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM bee_migrations`).Scan(&count); err != nil {
		t.Fatalf("counting bee_migrations rows: %v", err)
	}
	if count != len(migrations) {
		t.Errorf("bee_migrations row count: want %d, got %d", len(migrations), count)
	}
}

func TestInitDB_TasksTable(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','W','/','idle',1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages (id,session_key,platform,content,received_at,created_at,updated_at) VALUES ('m1','sk','p','c',1,1,1)`)

	_, err = db.Exec(`INSERT INTO bee_tasks
		(id, message_id, worker_id, instruction, type, created_at, updated_at)
		VALUES ('t1','m1','w1','do it','immediate',1,1)`)
	if err != nil {
		t.Fatalf("tasks table not created: %v", err)
	}
}

func TestMigrations_SkipsApplied(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	// Confirm version 1 was applied exactly once
	var countBefore int
	if err := db.QueryRow(`SELECT COUNT(*) FROM bee_migrations WHERE version = 1`).Scan(&countBefore); err != nil {
		t.Fatalf("querying bee_migrations: %v", err)
	}
	if countBefore != 1 {
		t.Fatalf("expected 1 row for version 1 before re-run, got %d", countBefore)
	}

	// Re-run migrate — version 1 should be skipped
	if err := migrate(db); err != nil {
		t.Fatalf("re-run migrate() failed: %v", err)
	}

	var countAfter int
	if err := db.QueryRow(`SELECT COUNT(*) FROM bee_migrations WHERE version = 1`).Scan(&countAfter); err != nil {
		t.Fatalf("querying bee_migrations after re-run: %v", err)
	}
	if countAfter != 1 {
		t.Errorf("version 1 should appear exactly once after re-run, got %d", countAfter)
	}
}

func TestMigration_UpgradesSessionContextsToPerEngineSchema(t *testing.T) {
	dbPath := t.TempDir() + "/legacy.db"
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE bee_migrations (
		version INTEGER PRIMARY KEY,
		name    TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create bee_migrations: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE bee_session_contexts (
		session_key TEXT NOT NULL,
		agent_id    TEXT NOT NULL,
		session_id  TEXT NOT NULL,
		updated_at  INTEGER NOT NULL,
		PRIMARY KEY (session_key, agent_id)
	)`); err != nil {
		t.Fatalf("create legacy session table: %v", err)
	}
	// bee_workers is required by migrations that run after migration 29.
	if _, err := db.Exec(`CREATE TABLE bee_workers (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		memory      TEXT NOT NULL DEFAULT '',
		work_dir    TEXT NOT NULL DEFAULT '',
		status      TEXT NOT NULL DEFAULT 'idle',
		created_at  INTEGER NOT NULL,
		updated_at  INTEGER NOT NULL
	)`); err != nil {
		t.Fatalf("create stub bee_workers: %v", err)
	}
	// bee_executions is required by migration 32 (index on started_at).
	if _, err := db.Exec(`CREATE TABLE bee_executions (
		id          TEXT PRIMARY KEY,
		worker_id   TEXT NOT NULL,
		session_id  TEXT NOT NULL,
		started_at  INTEGER NOT NULL,
		completed_at INTEGER,
		status      TEXT NOT NULL DEFAULT '',
		result      TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create stub bee_executions: %v", err)
	}
	// bee_outbound_messages is required by migration 34 (index on sent_at).
	if _, err := db.Exec(`CREATE TABLE bee_outbound_messages (
		id      TEXT PRIMARY KEY,
		sent_at INTEGER NOT NULL
	)`); err != nil {
		t.Fatalf("create stub bee_outbound_messages: %v", err)
	}
	// bee_platform_messages is required by migration 31 (drop retry_count).
	if _, err := db.Exec(`CREATE TABLE bee_platform_messages (
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
	)`); err != nil {
		t.Fatalf("create stub bee_platform_messages: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO bee_session_contexts (session_key, agent_id, session_id, updated_at)
		VALUES ('sk', 'bee', 'legacy-sid', 1)`); err != nil {
		t.Fatalf("seed legacy session row: %v", err)
	}
	for _, m := range migrations {
		if m.version >= 29 {
			break
		}
		if _, err := db.Exec(`INSERT INTO bee_migrations (version, name) VALUES (?, ?)`, m.version, m.name); err != nil {
			t.Fatalf("seed migration %d: %v", m.version, err)
		}
	}

	if err := migrate(db); err != nil {
		t.Fatalf("run migrate: %v", err)
	}

	var sessionID, engine string
	if err := db.QueryRow(`SELECT session_id, engine
		FROM bee_session_contexts
		WHERE session_key = 'sk' AND agent_id = 'bee' AND engine = ?`, ai.EngineClaude).Scan(&sessionID, &engine); err != nil {
		t.Fatalf("query migrated row: %v", err)
	}
	if sessionID != "legacy-sid" || engine != ai.EngineClaude {
		t.Fatalf("unexpected migrated row: session_id=%q engine=%q", sessionID, engine)
	}

	if _, err := db.Exec(`INSERT INTO bee_session_contexts (session_key, agent_id, session_id, updated_at, engine)
		VALUES ('sk', 'bee', 'codex-sid', 2, 'codex')`); err != nil {
		t.Fatalf("insert codex row after migration: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM bee_session_contexts WHERE session_key = 'sk' AND agent_id = 'bee'`).Scan(&count); err != nil {
		t.Fatalf("count migrated rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 per-engine rows after migration, got %d", count)
	}
}
