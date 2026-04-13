package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
	_ "modernc.org/sqlite"
)

type migration struct {
	version int
	name    string
	sql     string
}

var migrations = []migration{
	{
		version: 1,
		name:    "create_table_bee_workers",
		sql: `CREATE TABLE IF NOT EXISTS bee_workers (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	work_dir    TEXT NOT NULL,
	status      TEXT NOT NULL DEFAULT 'idle',
	description TEXT NOT NULL DEFAULT '',
	memory      TEXT NOT NULL DEFAULT '',
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
)`,
	},
	{
		version: 2,
		name:    "create_table_bee_executions",
		sql: `CREATE TABLE IF NOT EXISTS bee_executions (
	id             TEXT PRIMARY KEY,
	worker_id      TEXT,
	session_id     TEXT NOT NULL,
	status         TEXT NOT NULL DEFAULT 'pending',
	ai_process_pid INTEGER NOT NULL DEFAULT 0,
	trigger_input  TEXT NOT NULL DEFAULT '',
	result         TEXT NOT NULL DEFAULT '',
	log_path       TEXT NOT NULL DEFAULT '',
	started_at     INTEGER,
	completed_at   INTEGER
)`,
	},
	{
		version: 3,
		name:    "create_table_bee_platform_messages",
		sql: `CREATE TABLE IF NOT EXISTS bee_platform_messages (
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
)`,
	},
	{
		version: 4,
		name:    "create_table_bee_tasks",
		sql: `CREATE TABLE IF NOT EXISTS bee_tasks (
	id           TEXT PRIMARY KEY,
	message_id   TEXT NOT NULL REFERENCES bee_platform_messages(id),
	worker_id    TEXT NOT NULL REFERENCES bee_workers(id),
	instruction  TEXT NOT NULL,
	type         TEXT NOT NULL CHECK(type IN ('immediate','countdown','scheduled')),
	status       TEXT NOT NULL DEFAULT 'pending'
	                 CHECK(status IN ('pending','running','completed','failed','cancelled')),
	scheduled_at INTEGER,
	cron_expr    TEXT NOT NULL DEFAULT '',
	next_run_at  INTEGER,
	execution_id TEXT NOT NULL DEFAULT '',
	created_at   INTEGER NOT NULL,
	updated_at   INTEGER NOT NULL
)`,
	},
	{
		version: 5,
		name:    "create_table_bee_session_contexts",
		sql: `CREATE TABLE IF NOT EXISTS bee_session_contexts (
	session_key  TEXT    NOT NULL,
	agent_id     TEXT    NOT NULL,
	session_id   TEXT    NOT NULL,
	updated_at   INTEGER NOT NULL,
	PRIMARY KEY (session_key, agent_id)
)`,
	},
	{
		version: 6,
		name:    "create_table_bee_local_sessions",
		sql: `CREATE TABLE IF NOT EXISTS bee_local_sessions (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
)`,
	},
	{
		version: 7,
		name:    "create_table_bee_local_replies",
		sql: `CREATE TABLE IF NOT EXISTS bee_local_replies (
	id          TEXT PRIMARY KEY,
	session_key TEXT NOT NULL,
	content     TEXT NOT NULL,
	created_at  INTEGER NOT NULL
)`,
	},
	{
		version: 8,
		name:    "create_table_bee_memories",
		sql: `CREATE TABLE IF NOT EXISTS bee_memories (
	id         TEXT PRIMARY KEY,
	scope      TEXT NOT NULL,
	key        TEXT NOT NULL,
	value      TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	UNIQUE(scope, key)
)`,
	},
	{
		version: 9,
		name:    "create_index_executions_worker_id",
		sql:     `CREATE INDEX IF NOT EXISTS idx_executions_worker_id ON bee_executions(worker_id)`,
	},
	{
		version: 10,
		name:    "create_index_executions_session_id",
		sql:     `CREATE INDEX IF NOT EXISTS idx_executions_session_id ON bee_executions(session_id)`,
	},
	{
		version: 11,
		name:    "create_index_platform_messages_session",
		sql:     `CREATE INDEX IF NOT EXISTS idx_platform_messages_session ON bee_platform_messages(session_key, received_at DESC)`,
	},
	{
		version: 12,
		name:    "create_index_platform_messages_platform_msg_id",
		sql:     `CREATE UNIQUE INDEX IF NOT EXISTS idx_platform_messages_platform_msg_id ON bee_platform_messages(platform_msg_id) WHERE platform_msg_id != ''`,
	},
	{
		version: 13,
		name:    "create_index_tasks_status_type",
		sql:     `CREATE INDEX IF NOT EXISTS idx_tasks_status_type ON bee_tasks(status, type)`,
	},
	{
		version: 14,
		name:    "create_index_tasks_message_id",
		sql:     `CREATE INDEX IF NOT EXISTS idx_tasks_message_id ON bee_tasks(message_id)`,
	},
	{
		version: 15,
		name:    "create_index_tasks_worker_id",
		sql:     `CREATE INDEX IF NOT EXISTS idx_tasks_worker_id ON bee_tasks(worker_id)`,
	},
	{
		version: 16,
		name:    "create_index_local_replies_session_key",
		sql:     `CREATE INDEX IF NOT EXISTS idx_local_replies_session_key ON bee_local_replies(session_key)`,
	},
	{
		version: 17,
		name:    "create_table_bee_outbound_messages",
		sql: `CREATE TABLE IF NOT EXISTS bee_outbound_messages (
	id              TEXT PRIMARY KEY,
	session_key     TEXT NOT NULL,
	platform        TEXT NOT NULL,
	content         TEXT NOT NULL DEFAULT '',
	media_path      TEXT NOT NULL DEFAULT '',
	status          TEXT NOT NULL DEFAULT 'sent' CHECK(status IN ('sent','failed')),
	platform_msg_id TEXT NOT NULL DEFAULT '',
	source_type     TEXT NOT NULL DEFAULT '' CHECK(source_type IN ('','bee','worker','system')),
	source_id       TEXT NOT NULL DEFAULT '',
	inbound_msg_id  TEXT NOT NULL DEFAULT '',
	error           TEXT NOT NULL DEFAULT '',
	retry_count     INTEGER NOT NULL DEFAULT 0,
	sent_at         INTEGER NOT NULL,
	created_at      INTEGER NOT NULL
)`,
	},
	{
		version: 18,
		name:    "create_index_outbound_session_key_sent_at",
		sql:     `CREATE INDEX IF NOT EXISTS idx_outbound_session_key_sent_at ON bee_outbound_messages(session_key, sent_at DESC)`,
	},
	{
		version: 19,
		name:    "create_index_outbound_platform_sent_at",
		sql:     `CREATE INDEX IF NOT EXISTS idx_outbound_platform_sent_at ON bee_outbound_messages(platform, sent_at DESC)`,
	},
	{
		version: 20,
		name:    "create_index_outbound_source_id_sent_at",
		sql:     `CREATE INDEX IF NOT EXISTS idx_outbound_source_id_sent_at ON bee_outbound_messages(source_id, sent_at DESC) WHERE source_id != ''`,
	},
	{
		version: 21,
		name:    "migrate_local_replies_to_outbound_messages",
		sql: `INSERT OR IGNORE INTO bee_outbound_messages (id, session_key, platform, content, status, sent_at, created_at)
SELECT id, session_key, 'local', content, 'sent', created_at, created_at
FROM bee_local_replies`,
	},
	{
		version: 22,
		name:    "create_table_bee_departments",
		sql: `CREATE TABLE IF NOT EXISTS bee_departments (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    parent_id  TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
)`,
	},
	{
		version: 23,
		name:    "create_index_departments_parent_id",
		sql:     `CREATE INDEX IF NOT EXISTS idx_departments_parent_id ON bee_departments(parent_id)`,
	},
	{
		version: 24,
		name:    "create_table_bee_worker_departments",
		sql: `CREATE TABLE IF NOT EXISTS bee_worker_departments (
    worker_id     TEXT NOT NULL REFERENCES bee_workers(id),
    department_id TEXT NOT NULL REFERENCES bee_departments(id),
    created_at    INTEGER NOT NULL,
    PRIMARY KEY (worker_id, department_id)
)`,
	},
	{
		version: 25,
		name:    "create_index_worker_depts_worker",
		sql:     `CREATE INDEX IF NOT EXISTS idx_worker_depts_worker ON bee_worker_departments(worker_id)`,
	},
	{
		version: 26,
		name:    "create_index_worker_depts_dept",
		sql:     `CREATE INDEX IF NOT EXISTS idx_worker_depts_dept ON bee_worker_departments(department_id)`,
	},
	{
		version: 27,
		name:    "deprecate_bee_local_sessions",
		sql: `-- bee_local_sessions is deprecated. Local chat now uses the fixed
-- session_key "local:default" and no longer reads or writes this table.
-- The table is preserved for historical data only.
SELECT 1`,
	},
	{
		version: 28,
		name:    "create_index_workers_name_lower",
		sql:     `CREATE INDEX IF NOT EXISTS idx_workers_name_lower ON bee_workers (LOWER(name))`,
	},
	{
		version: 29,
		name:    "migrate_bee_session_contexts_to_engine_keyed_schema",
		sql: fmt.Sprintf(`CREATE TABLE bee_session_contexts_new (
	session_key TEXT NOT NULL,
	agent_id    TEXT NOT NULL,
	engine      TEXT NOT NULL,
	session_id  TEXT NOT NULL,
	updated_at  INTEGER NOT NULL,
	PRIMARY KEY (session_key, agent_id, engine)
);
INSERT INTO bee_session_contexts_new (session_key, agent_id, engine, session_id, updated_at)
SELECT session_key, agent_id, '%s', session_id, updated_at
FROM bee_session_contexts;
DROP TABLE bee_session_contexts;
ALTER TABLE bee_session_contexts_new RENAME TO bee_session_contexts;`, ai.EngineClaude),
	},
}

// stringsToArgs converts a string slice to a []any slice for use as SQL query arguments.
func stringsToArgs(ss []string) []any {
	args := make([]any, len(ss))
	for i, s := range ss {
		args[i] = s
	}
	return args
}

// inPlaceholders returns n comma-separated "?" for SQL IN clauses, e.g. inPlaceholders(3) == "?,?,?".
func inPlaceholders(n int) string {
	if n == 0 {
		return ""
	}
	ph := strings.Repeat("?,", n)
	return ph[:len(ph)-1]
}

func InitDB(dbPath string) (*sql.DB, error) {
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS bee_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	)`); err != nil {
		return err
	}
	return applyMigrations(db, migrations)
}

func applyMigrations(db *sql.DB, migrations []migration) error {
	for _, m := range migrations {
		var count int
		err := db.QueryRow(`SELECT COUNT(*) FROM bee_migrations WHERE version = ?`, m.version).Scan(&count)
		if err != nil {
			return fmt.Errorf("checking migration %d: %w", m.version, err)
		}
		if count > 0 {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for migration %d: %w", m.version, err)
		}
		if _, err = tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d %q: %w", m.version, m.name, err)
		}
		if _, err = tx.Exec(`INSERT INTO bee_migrations (version, name) VALUES (?, ?)`, m.version, m.name); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", m.version, err)
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
	}
	return nil
}
