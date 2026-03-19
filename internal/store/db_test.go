package store

import (
	"os"
	"testing"
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
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version).Scan(&count); err != nil {
			t.Fatalf("querying schema_migrations for version %d: %v", m.version, err)
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
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'`).Scan(&name); err != nil {
		t.Fatalf("schema_migrations table not found: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("counting schema_migrations rows: %v", err)
	}
	if count != len(migrations) {
		t.Errorf("schema_migrations row count: want %d, got %d", len(migrations), count)
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
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 1`).Scan(&countBefore); err != nil {
		t.Fatalf("querying schema_migrations: %v", err)
	}
	if countBefore != 1 {
		t.Fatalf("expected 1 row for version 1 before re-run, got %d", countBefore)
	}

	// Re-run migrate — version 1 should be skipped
	if err := migrate(db); err != nil {
		t.Fatalf("re-run migrate() failed: %v", err)
	}

	var countAfter int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 1`).Scan(&countAfter); err != nil {
		t.Fatalf("querying schema_migrations after re-run: %v", err)
	}
	if countAfter != 1 {
		t.Errorf("version 1 should appear exactly once after re-run, got %d", countAfter)
	}
}
