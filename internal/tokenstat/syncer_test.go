package tokenstat_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/tokenstat"
)

func newSyncerTestDB(t *testing.T) (*sql.DB, *store.TokenStatsStore, func()) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	return db, store.NewTokenStatsStore(db), func() { db.Close() }
}

func insertTestWorker(t *testing.T, db *sql.DB, id, engine string) {
	t.Helper()
	now := time.Now().UnixMilli()
	_, err := db.Exec(
		`INSERT INTO bee_workers (id, name, description, constraints, work_dir, engine, status, permission_scopes, created_at, updated_at)
		 VALUES (?, ?, '', '', '/tmp', ?, 'idle', '', ?, ?)`,
		id, "worker-"+id, engine, now, now,
	)
	if err != nil {
		t.Fatalf("insert worker: %v", err)
	}
}

func insertTestExecution(t *testing.T, db *sql.DB, workerID, sessionID string, completedAt int64) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO bee_executions (id, worker_id, session_id, engine, status, completed_at)
		 VALUES (?, ?, ?, '', 'completed', ?)`,
		"exec-"+sessionID, workerID, sessionID, completedAt,
	)
	if err != nil {
		t.Fatalf("insert execution: %v", err)
	}
}

func insertTestExecutionWithEngine(t *testing.T, db *sql.DB, workerID, sessionID, engine string, completedAt int64) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO bee_executions (id, worker_id, session_id, engine, status, completed_at)
		 VALUES (?, ?, ?, ?, 'completed', ?)`,
		"exec-"+sessionID, workerID, sessionID, engine, completedAt,
	)
	if err != nil {
		t.Fatalf("insert execution with engine: %v", err)
	}
}

func TestSyncer_SyncOnce_Claude(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "worker-1", "claude")
	insertTestExecution(t, db, "worker-1", "test-session", time.Now().UnixMilli())

	claudeBase := t.TempDir()
	os.MkdirAll(filepath.Join(claudeBase, "projects"), 0755)
	os.WriteFile(
		filepath.Join(claudeBase, "projects", "test-session.jsonl"),
		[]byte(`{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":100,"output_tokens":50}}}`+"\n"),
		0644,
	)
	t.Setenv("CLAUDE_CONFIG_DIR", claudeBase)

	syncer := tokenstat.NewSyncer(db, tokenStore)
	syncer.SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("test-session")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat record, got %d", len(stats))
	}
	if stats[0].InputTokens != 100 {
		t.Errorf("InputTokens: want 100, got %d", stats[0].InputTokens)
	}
	if stats[0].AgentType != "claude" {
		t.Errorf("AgentType: want claude, got %s", stats[0].AgentType)
	}
}

func TestSyncer_SyncOnce_FullModeWhenTableEmpty(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "worker-old", "claude")
	oldTime := time.Now().AddDate(0, 0, -60).UnixMilli()
	insertTestExecution(t, db, "worker-old", "old-session", oldTime)

	claudeBase := t.TempDir()
	os.MkdirAll(filepath.Join(claudeBase, "projects"), 0755)
	os.WriteFile(
		filepath.Join(claudeBase, "projects", "old-session.jsonl"),
		[]byte(`{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":50,"output_tokens":25}}}`+"\n"),
		0644,
	)
	t.Setenv("CLAUDE_CONFIG_DIR", claudeBase)

	syncer := tokenstat.NewSyncer(db, tokenStore)
	syncer.SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("old-session")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(stats) != 1 {
		t.Errorf("expected 1 stat (full mode on empty table), got %d", len(stats))
	}
}

func TestSyncer_SyncOnce_RetriesUnsyncedHistoricalSessionsAfterPartialBackfill(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	if err := tokenStore.Upsert(model.TokenStats{
		SessionID:   "seed-session",
		AgentType:   "claude",
		Model:       "claude-3-5-sonnet",
		InputTokens: 1,
		SyncedAt:    time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("seed token stats: %v", err)
	}

	insertTestWorker(t, db, "worker-old", "claude")
	oldTime := time.Now().AddDate(0, 0, -60).UnixMilli()
	insertTestExecution(t, db, "worker-old", "old-unsynced-session", oldTime)

	claudeBase := t.TempDir()
	os.MkdirAll(filepath.Join(claudeBase, "projects"), 0755)
	os.WriteFile(
		filepath.Join(claudeBase, "projects", "old-unsynced-session.jsonl"),
		[]byte(`{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":75,"output_tokens":35}}}`+"\n"),
		0644,
	)
	t.Setenv("CLAUDE_CONFIG_DIR", claudeBase)

	syncer := tokenstat.NewSyncer(db, tokenStore)
	syncer.SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("old-unsynced-session")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected unsynced historical session to be retried, got %d stats", len(stats))
	}
	if stats[0].InputTokens != 75 {
		t.Fatalf("InputTokens: want 75, got %d", stats[0].InputTokens)
	}
}

func TestSyncer_SyncOnce_UsesExecutionEngineHint(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "worker-1", "codex")
	insertTestExecutionWithEngine(t, db, "worker-1", "claude-session", "claude", time.Now().UnixMilli())

	claudeBase := t.TempDir()
	os.MkdirAll(filepath.Join(claudeBase, "projects"), 0755)
	os.WriteFile(
		filepath.Join(claudeBase, "projects", "claude-session.jsonl"),
		[]byte(`{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":120,"output_tokens":60}}}`+"\n"),
		0644,
	)
	t.Setenv("CLAUDE_CONFIG_DIR", claudeBase)

	syncer := tokenstat.NewSyncer(db, tokenStore)
	syncer.SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("claude-session")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat record, got %d", len(stats))
	}
	if stats[0].AgentType != "claude" {
		t.Fatalf("AgentType: want claude, got %s", stats[0].AgentType)
	}
}

func TestSyncer_SyncOnce_Kimi(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "worker-kimi", "kimi")
	insertTestExecutionWithEngine(t, db, "worker-kimi", "kimi-sess-001", "kimi", time.Now().UnixMilli())

	home := t.TempDir()
	kimiDir := filepath.Join(home, ".kimi", "sessions", "bucket-01", "kimi-sess-001")
	os.MkdirAll(kimiDir, 0755)
	os.WriteFile(filepath.Join(kimiDir, "wire.jsonl"),
		[]byte(`{"message":{"type":"StatusUpdate","payload":{"token_usage":{"input_other":446,"output":70,"input_cache_read":16384,"input_cache_creation":0}}}}`+"\n"),
		0644,
	)
	t.Setenv("HOME", home)

	syncer := tokenstat.NewSyncer(db, tokenStore)
	syncer.SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("kimi-sess-001")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat record, got %d", len(stats))
	}
	if stats[0].AgentType != "kimi" {
		t.Errorf("AgentType: want kimi, got %s", stats[0].AgentType)
	}
	if stats[0].Model != "kimi" {
		t.Errorf("Model: want kimi, got %s", stats[0].Model)
	}
	if stats[0].InputTokens != 446 {
		t.Errorf("InputTokens: want 446, got %d", stats[0].InputTokens)
	}
	if stats[0].OutputTokens != 70 {
		t.Errorf("OutputTokens: want 70, got %d", stats[0].OutputTokens)
	}
	if stats[0].CacheReadTokens != 16384 {
		t.Errorf("CacheReadTokens: want 16384, got %d", stats[0].CacheReadTokens)
	}
}

func TestSyncer_SyncOnce_LegacyExecutionWithoutEngineFallsBackAcrossParsers(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "worker-1", "kimi")
	insertTestExecution(t, db, "worker-1", "legacy-claude-session", time.Now().UnixMilli())

	claudeBase := t.TempDir()
	os.MkdirAll(filepath.Join(claudeBase, "projects"), 0755)
	os.WriteFile(
		filepath.Join(claudeBase, "projects", "legacy-claude-session.jsonl"),
		[]byte(`{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":90,"output_tokens":45}}}`+"\n"),
		0644,
	)
	t.Setenv("CLAUDE_CONFIG_DIR", claudeBase)

	syncer := tokenstat.NewSyncer(db, tokenStore)
	syncer.SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("legacy-claude-session")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat record, got %d", len(stats))
	}
	if stats[0].InputTokens != 90 {
		t.Fatalf("InputTokens: want 90, got %d", stats[0].InputTokens)
	}
}
