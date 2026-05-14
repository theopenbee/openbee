package tokenstat_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/bridge"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/tokenstat"
)

type collectCall struct {
	sessionID string
	engine    string
}

// fakeBridge is a test double for bridge.Bridge.
type fakeBridge struct {
	collect func(ctx context.Context, sessionID, engine string) (bridge.UsageResult, error)
	calls   []collectCall
}

func (f *fakeBridge) EnabledEngines() []string                             { return nil }
func (f *fakeBridge) ValidateEngine(string) error                          { return nil }
func (f *fakeBridge) ResolveEngine(engine string) (string, error)          { return engine, nil }
func (f *fakeBridge) BuildBeeSessionPrefix() string                        { return "" }
func (f *fakeBridge) BuildWorkerSessionPrefix(bridge.WorkerPersona) string { return "" }
func (f *fakeBridge) PrepareBeeWorkspace(string) error                     { return nil }
func (f *fakeBridge) PrepareWorkerWorkspace(string, string) error          { return nil }
func (f *fakeBridge) RunBee(context.Context, bridge.BeeRunRequest) (bridge.RunHandle, error) {
	return bridge.RunHandle{}, nil
}
func (f *fakeBridge) RunWorker(context.Context, bridge.WorkerRunRequest) (bridge.RunHandle, error) {
	return bridge.RunHandle{}, nil
}
func (f *fakeBridge) CollectTokenUsage(ctx context.Context, sessionID, engine string) (bridge.UsageResult, error) {
	f.calls = append(f.calls, collectCall{sessionID: sessionID, engine: engine})
	return f.collect(ctx, sessionID, engine)
}

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

func insertTestExecutionWithEngine(t *testing.T, db *sql.DB, workerID, sessionID, engine string, completedAt int64) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO bee_executions (id, worker_id, session_id, engine, status, completed_at)
		 VALUES (?, ?, ?, ?, 'completed', ?)`,
		"exec-"+sessionID, workerID, sessionID, engine, completedAt,
	)
	if err != nil {
		t.Fatalf("insert execution: %v", err)
	}
}

func insertTestExecution(t *testing.T, db *sql.DB, workerID, sessionID string, completedAt int64) {
	insertTestExecutionWithEngine(t, db, workerID, sessionID, "", completedAt)
}

// TestSyncer_Direct_KnownEngine: known engine adapter returns usages → row written with correct AgentType.
func TestSyncer_Direct_KnownEngine(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "w1", "claude")
	insertTestExecutionWithEngine(t, db, "w1", "sess-1", "claude", time.Now().UnixMilli())

	br := &fakeBridge{collect: func(_ context.Context, _ string, _ string) (bridge.UsageResult, error) {
		return bridge.UsageResult{
			Engine: bridge.EngineClaude,
			Usages: []bridge.TokenUsage{{Model: "sonnet-4", InputTokens: 100, OutputTokens: 50}},
		}, nil
	}}
	syncer := tokenstat.NewSyncer(db, tokenStore, br)
	syncer.SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("sess-1")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat, got %d", len(stats))
	}
	if stats[0].Model != "sonnet-4" {
		t.Errorf("Model: want sonnet-4, got %s", stats[0].Model)
	}
	if stats[0].InputTokens != 100 {
		t.Errorf("InputTokens: want 100, got %d", stats[0].InputTokens)
	}
	if stats[0].AgentType != "claude" {
		t.Errorf("AgentType: want claude, got %s", stats[0].AgentType)
	}
	if len(br.calls) != 1 || br.calls[0].engine != bridge.EngineClaude {
		t.Fatalf("CollectTokenUsage calls=%+v", br.calls)
	}
}

// TestSyncer_Direct_KnownEngine_NotFound_Tombstones: known engine returns ErrSessionDataNotFound → tombstone.
func TestSyncer_Direct_KnownEngine_NotFound_Tombstones(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "w1", "claude")
	insertTestExecutionWithEngine(t, db, "w1", "sess-nf", "claude", time.Now().UnixMilli())

	br := &fakeBridge{collect: func(_ context.Context, _ string, _ string) (bridge.UsageResult, error) {
		return bridge.UsageResult{}, bridge.ErrSessionDataNotFound
	}}
	syncer := tokenstat.NewSyncer(db, tokenStore, br)
	syncer.SyncOnce(context.Background())

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM bee_token_stats WHERE session_id = ? AND model = 'unknown'`, "sess-nf").Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 tombstone, got %d", count)
	}
	// Tombstone must not appear in the public API.
	if stats, err := tokenStore.GetBySessionID("sess-nf"); err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	} else if len(stats) != 0 {
		t.Errorf("tombstone must not appear in API query, got %d records", len(stats))
	}
}

// TestSyncer_Direct_KnownEngine_Empty_Tombstones: known engine returns ([], nil) → tombstone.
func TestSyncer_Direct_KnownEngine_Empty_Tombstones(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "w1", "claude")
	insertTestExecutionWithEngine(t, db, "w1", "sess-empty", "claude", time.Now().UnixMilli())

	br := &fakeBridge{collect: func(_ context.Context, _ string, _ string) (bridge.UsageResult, error) {
		return bridge.UsageResult{Engine: bridge.EngineClaude}, nil
	}}
	syncer := tokenstat.NewSyncer(db, tokenStore, br)
	syncer.SyncOnce(context.Background())

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM bee_token_stats WHERE session_id = ? AND model = 'unknown'`, "sess-empty").Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 tombstone for empty usages, got %d", count)
	}
}

// TestSyncer_Direct_KnownEngine_HardError_NoTombstone: non-NotFound error → no row written, session left pending.
func TestSyncer_Direct_KnownEngine_HardError_NoTombstone(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "w1", "claude")
	insertTestExecutionWithEngine(t, db, "w1", "sess-err", "claude", time.Now().UnixMilli())

	br := &fakeBridge{collect: func(_ context.Context, _ string, _ string) (bridge.UsageResult, error) {
		return bridge.UsageResult{}, context.DeadlineExceeded
	}}
	syncer := tokenstat.NewSyncer(db, tokenStore, br)
	syncer.SyncOnce(context.Background())

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM bee_token_stats WHERE session_id = ?`, "sess-err").Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows on hard error, got %d", count)
	}
}

// TestSyncer_Legacy_FallbackHits: engine empty, bridge resolves usage to the hit engine.
func TestSyncer_Legacy_FallbackHits(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "w1", "")
	insertTestExecution(t, db, "w1", "sess-legacy", time.Now().UnixMilli())

	br := &fakeBridge{collect: func(_ context.Context, _ string, engine string) (bridge.UsageResult, error) {
		if engine != "" {
			t.Fatalf("legacy session should pass empty engine to bridge, got %q", engine)
		}
		return bridge.UsageResult{
			Engine: bridge.EngineKimi,
			Usages: []bridge.TokenUsage{{Model: "kimi", InputTokens: 200}},
		}, nil
	}}
	syncer := tokenstat.NewSyncer(db, tokenStore, br)
	syncer.SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("sess-legacy")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat, got %d", len(stats))
	}
	if stats[0].InputTokens != 200 {
		t.Errorf("InputTokens: want 200, got %d", stats[0].InputTokens)
	}
	if stats[0].AgentType != "kimi" {
		t.Errorf("AgentType: want kimi, got %s", stats[0].AgentType)
	}
}

// TestSyncer_Legacy_AllNotFound_Tombstones: engine empty, bridge returns NotFound -> tombstone.
func TestSyncer_Legacy_AllNotFound_Tombstones(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "w1", "")
	insertTestExecution(t, db, "w1", "sess-all-nf", time.Now().UnixMilli())

	br := &fakeBridge{collect: func(_ context.Context, _ string, _ string) (bridge.UsageResult, error) {
		return bridge.UsageResult{}, bridge.ErrSessionDataNotFound
	}}
	syncer := tokenstat.NewSyncer(db, tokenStore, br)
	syncer.SyncOnce(context.Background())

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM bee_token_stats WHERE session_id = ? AND model = 'unknown'`, "sess-all-nf").Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 tombstone, got %d", count)
	}
	// Second SyncOnce must not produce a second tombstone.
	syncer.SyncOnce(context.Background())
	if err := db.QueryRow(`SELECT COUNT(*) FROM bee_token_stats WHERE session_id = ?`, "sess-all-nf").Scan(&count); err != nil {
		t.Fatalf("query after 2nd sync: %v", err)
	}
	if count != 1 {
		t.Errorf("expected still 1 record after second sync, got %d", count)
	}
}

// TestSyncer_UnknownEngine_FallsBack: unknown engine is delegated to bridge, which resolves fallback.
func TestSyncer_UnknownEngine_FallsBack(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "w1", "")
	insertTestExecutionWithEngine(t, db, "w1", "sess-unknown", "obsolete-engine", time.Now().UnixMilli())

	br := &fakeBridge{collect: func(_ context.Context, _ string, _ string) (bridge.UsageResult, error) {
		return bridge.UsageResult{
			Engine: bridge.EngineClaude,
			Usages: []bridge.TokenUsage{{Model: "sonnet-4", InputTokens: 77}},
		}, nil
	}}
	syncer := tokenstat.NewSyncer(db, tokenStore, br)
	syncer.SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("sess-unknown")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat, got %d", len(stats))
	}
	if stats[0].InputTokens != 77 {
		t.Errorf("InputTokens: want 77, got %d", stats[0].InputTokens)
	}
	if len(br.calls) != 1 || br.calls[0].engine != "obsolete-engine" {
		t.Fatalf("CollectTokenUsage calls=%+v", br.calls)
	}
}

// TestSyncer_UnknownEngine_AllNotFound_Tombstones: bridge returns NotFound -> tombstone.
func TestSyncer_UnknownEngine_AllNotFound_Tombstones(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "w1", "")
	insertTestExecutionWithEngine(t, db, "w1", "sess-unk-nf", "obsolete-engine", time.Now().UnixMilli())

	br := &fakeBridge{collect: func(_ context.Context, _ string, _ string) (bridge.UsageResult, error) {
		return bridge.UsageResult{}, bridge.ErrSessionDataNotFound
	}}
	syncer := tokenstat.NewSyncer(db, tokenStore, br)
	syncer.SyncOnce(context.Background())

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM bee_token_stats WHERE session_id = ? AND model = 'unknown'`, "sess-unk-nf").Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 tombstone, got %d", count)
	}
}

// TestSyncer_DoesNotResyncCompleted: session with synced_at > completed_at is skipped.
func TestSyncer_DoesNotResyncCompleted(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "w1", "claude")
	completedAt := time.Now().Add(-1 * time.Hour).UnixMilli()
	insertTestExecutionWithEngine(t, db, "w1", "sess-done", "claude", completedAt)

	// Seed a token stat with synced_at > completed_at so the SQL HAVING clause skips it.
	if err := tokenStore.Upsert(model.TokenStats{
		SessionID:   "sess-done",
		AgentType:   "claude",
		Model:       "sonnet-4",
		InputTokens: 42,
		SyncedAt:    time.Now().UnixMilli(), // > completedAt
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	callCount := 0
	br := &fakeBridge{collect: func(_ context.Context, _ string, _ string) (bridge.UsageResult, error) {
		callCount++
		return bridge.UsageResult{
			Engine: bridge.EngineClaude,
			Usages: []bridge.TokenUsage{{Model: "sonnet-4", InputTokens: 999}},
		}, nil
	}}
	syncer := tokenstat.NewSyncer(db, tokenStore, br)
	syncer.SyncOnce(context.Background())

	if callCount != 0 {
		t.Errorf("adapter should not be called for already-synced session, got %d calls", callCount)
	}
	// Original value must be unchanged.
	stats, err := tokenStore.GetBySessionID("sess-done")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if len(stats) != 1 || stats[0].InputTokens != 42 {
		t.Errorf("expected unchanged InputTokens=42, got %+v", stats)
	}
}
