package store

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type migration struct {
	version int
	name    string
	sql     string
}

var migrations = []migration{
	{
		version: 1,
		name:    "20260312_create_table_workers",
		sql: `CREATE TABLE IF NOT EXISTS workers (
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
		name:    "20260312_create_table_worker_executions",
		sql: `CREATE TABLE IF NOT EXISTS worker_executions (
		id             TEXT PRIMARY KEY,
		worker_id      TEXT NOT NULL,
		session_id     TEXT NOT NULL,
		status         TEXT NOT NULL DEFAULT 'pending',
		ai_process_pid INTEGER NOT NULL DEFAULT 0,
		trigger_input  TEXT NOT NULL DEFAULT '',
		result         TEXT NOT NULL DEFAULT '',
		logs           TEXT NOT NULL DEFAULT '',
		started_at     INTEGER,
		completed_at   INTEGER,
		FOREIGN KEY (worker_id) REFERENCES workers(id) ON DELETE CASCADE
	)`,
	},
	{
		version: 3,
		name:    "20260312_create_table_platform_messages",
		sql: `CREATE TABLE IF NOT EXISTS platform_messages (
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
		updated_at      INTEGER NOT NULL
	)`,
	},
	{
		version: 4,
		name:    "20260312_create_table_tasks",
		sql: `CREATE TABLE IF NOT EXISTS tasks (
		id                TEXT PRIMARY KEY,
		message_id        TEXT NOT NULL REFERENCES platform_messages(id),
		worker_id         TEXT NOT NULL REFERENCES workers(id),
		instruction       TEXT NOT NULL,
		type              TEXT NOT NULL CHECK(type IN ('immediate','countdown','scheduled')),
		status            TEXT NOT NULL DEFAULT 'pending'
		                      CHECK(status IN ('pending','running','completed','failed','cancelled')),
		scheduled_at      INTEGER,
		cron_expr         TEXT NOT NULL DEFAULT '',
		next_run_at       INTEGER,
		execution_id      TEXT NOT NULL DEFAULT '',
		created_at        INTEGER NOT NULL,
		updated_at        INTEGER NOT NULL
	)`,
	},
	{
		version: 5,
		name:    "20260312_create_index_worker_executions_worker_id",
		sql:     `CREATE INDEX IF NOT EXISTS idx_worker_executions_worker_id ON worker_executions(worker_id)`,
	},
	{
		version: 6,
		name:    "20260312_create_index_worker_executions_session_id",
		sql:     `CREATE INDEX IF NOT EXISTS idx_worker_executions_session_id ON worker_executions(session_id)`,
	},
	{
		version: 7,
		name:    "20260312_create_index_platform_messages_session",
		sql:     `CREATE INDEX IF NOT EXISTS idx_platform_messages_session ON platform_messages(session_key, received_at DESC)`,
	},
	{
		version: 8,
		name:    "20260312_create_unique_index_platform_messages_platform_msg_id",
		sql:     `CREATE UNIQUE INDEX IF NOT EXISTS idx_platform_messages_platform_msg_id ON platform_messages(platform_msg_id) WHERE platform_msg_id != ''`,
	},
	{
		version: 9,
		name:    "20260312_create_index_tasks_status_type",
		sql:     `CREATE INDEX IF NOT EXISTS idx_tasks_status_type ON tasks(status, type)`,
	},
	{
		version: 10,
		name:    "20260312_create_index_tasks_message_id",
		sql:     `CREATE INDEX IF NOT EXISTS idx_tasks_message_id ON tasks(message_id)`,
	},
	{
		version: 11,
		name:    "20260312_create_index_tasks_worker_id",
		sql:     `CREATE INDEX IF NOT EXISTS idx_tasks_worker_id ON tasks(worker_id)`,
	},
	{
		version: 12,
		name:    "20260312_create_table_session_contexts",
		sql: `CREATE TABLE IF NOT EXISTS session_contexts (
		session_key  TEXT    NOT NULL,
		agent_id     TEXT    NOT NULL,
		session_id   TEXT    NOT NULL,
		updated_at   INTEGER NOT NULL,
		PRIMARY KEY (session_key, agent_id)
	)`,
	},
}

func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000")
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
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
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
		err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version).Scan(&count)
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
		if _, err = tx.Exec(`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`, m.version, m.name); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", m.version, err)
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
	}
	return nil
}
