package tokenstat_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/tokenstat"
)

// fakeAdapter is a test double for ai.EngineAdapter.
type fakeAdapter struct {
	collect func(ctx context.Context, sessionID string) ([]ai.TokenUsage, error)
}

func (f *fakeAdapter) Prepare(string, ai.PrepareOptions) error { return nil }
func (f *fakeAdapter) Run(context.Context, string, string, ai.RunOptions, string) (ai.RunResult, error) {
	return ai.RunResult{}, nil
}
func (f *fakeAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return f.collect(ctx, sessionID)
}

func newSyncerTestDB(t *testing.T) (*sql.DB, *store.TokenStatsStore, func()) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
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
	require.NoError(t, err)
}

func insertTestExecutionWithEngine(t *testing.T, db *sql.DB, workerID, sessionID, engine string, completedAt int64) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO bee_executions (id, worker_id, session_id, engine, status, completed_at)
		 VALUES (?, ?, ?, ?, 'completed', ?)`,
		"exec-"+sessionID, workerID, sessionID, engine, completedAt,
	)
	require.NoError(t, err)
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

	adapters := map[string]ai.EngineAdapter{
		ai.EngineClaude: &fakeAdapter{collect: func(_ context.Context, _ string) ([]ai.TokenUsage, error) {
			return []ai.TokenUsage{{Model: "sonnet-4", InputTokens: 100, OutputTokens: 50}}, nil
		}},
	}
	syncer := tokenstat.NewSyncer(db, tokenStore, adapters, ai.AllEngines())
	syncer.SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("sess-1")
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, "sonnet-4", stats[0].Model)
	assert.EqualValues(t, 100, stats[0].InputTokens)
	assert.Equal(t, "claude", stats[0].AgentType)
}

// TestSyncer_Direct_KnownEngine_Tombstones merges the three tombstone-vs-no-tombstone
// scenarios of the known-engine direct path: NotFound and Empty usages leave a
// tombstone row, while a hard (non-NotFound) error leaves nothing written.
//
// Named with a "_Tombstones" suffix (rather than exactly "TestSyncer_Direct_KnownEngine")
// to avoid colliding with the pre-existing success-path test of that name above.
func TestSyncer_Direct_KnownEngine_Tombstones(t *testing.T) {
	cases := []struct {
		name           string
		sessionID      string
		usage          []ai.TokenUsage
		err            error
		wantTombstone  bool
		checkAPIHidden bool // also assert the tombstone is invisible via GetBySessionID
	}{
		{
			name:           "NotFound_Tombstones",
			sessionID:      "sess-nf",
			usage:          nil,
			err:            ai.ErrSessionDataNotFound,
			wantTombstone:  true,
			checkAPIHidden: true,
		},
		{
			name:          "Empty_Tombstones",
			sessionID:     "sess-empty",
			usage:         []ai.TokenUsage{},
			err:           nil,
			wantTombstone: true,
		},
		{
			name:          "HardError_NoTombstone",
			sessionID:     "sess-err",
			usage:         nil,
			err:           context.DeadlineExceeded,
			wantTombstone: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, tokenStore, cleanup := newSyncerTestDB(t)
			defer cleanup()

			insertTestWorker(t, db, "w1", "claude")
			insertTestExecutionWithEngine(t, db, "w1", tc.sessionID, "claude", time.Now().UnixMilli())

			adapters := map[string]ai.EngineAdapter{
				ai.EngineClaude: &fakeAdapter{collect: func(_ context.Context, _ string) ([]ai.TokenUsage, error) {
					return tc.usage, tc.err
				}},
			}
			syncer := tokenstat.NewSyncer(db, tokenStore, adapters, ai.AllEngines())
			syncer.SyncOnce(context.Background())

			var count int
			var query string
			wantCount := 0
			if tc.wantTombstone {
				query = `SELECT COUNT(*) FROM bee_token_stats WHERE session_id = ? AND model = 'unknown'`
				wantCount = 1
			} else {
				query = `SELECT COUNT(*) FROM bee_token_stats WHERE session_id = ?`
			}
			require.NoError(t, db.QueryRow(query, tc.sessionID).Scan(&count))
			if tc.wantTombstone {
				// Originals for both tombstone cases used t.Fatalf here.
				require.Equal(t, wantCount, count)
			} else {
				// HardError_NoTombstone's original used t.Errorf here.
				assert.Equal(t, wantCount, count)
			}

			if tc.checkAPIHidden {
				// Tombstone must not appear in the public API.
				stats, err := tokenStore.GetBySessionID(tc.sessionID)
				require.NoError(t, err)
				assert.Empty(t, stats)
			}
		})
	}
}

// TestSyncer_Legacy_FallbackHits: engine empty, third adapter in chain succeeds.
func TestSyncer_Legacy_FallbackHits(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "w1", "")
	insertTestExecution(t, db, "w1", "sess-legacy", time.Now().UnixMilli())

	notFound := &fakeAdapter{collect: func(_ context.Context, _ string) ([]ai.TokenUsage, error) {
		return nil, ai.ErrSessionDataNotFound
	}}
	piHit := &fakeAdapter{collect: func(_ context.Context, _ string) ([]ai.TokenUsage, error) {
		return []ai.TokenUsage{{Model: "pi", InputTokens: 200}}, nil
	}}
	adapters := map[string]ai.EngineAdapter{
		ai.EngineClaude: notFound,
		ai.EngineCodex:  notFound,
		ai.EnginePi:     piHit,
	}
	syncer := tokenstat.NewSyncer(db, tokenStore, adapters, ai.AllEngines())
	syncer.SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("sess-legacy")
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.EqualValues(t, 200, stats[0].InputTokens)
	assert.Equal(t, "pi", stats[0].AgentType)
}

// TestSyncer_Legacy_AllNotFound_Tombstones: engine empty, all adapters return NotFound → tombstone.
func TestSyncer_Legacy_AllNotFound_Tombstones(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "w1", "")
	insertTestExecution(t, db, "w1", "sess-all-nf", time.Now().UnixMilli())

	notFound := &fakeAdapter{collect: func(_ context.Context, _ string) ([]ai.TokenUsage, error) {
		return nil, ai.ErrSessionDataNotFound
	}}
	adapters := map[string]ai.EngineAdapter{
		ai.EngineClaude: notFound,
		ai.EngineCodex:  notFound,
		ai.EnginePi:     notFound,
	}
	syncer := tokenstat.NewSyncer(db, tokenStore, adapters, ai.AllEngines())
	syncer.SyncOnce(context.Background())

	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM bee_token_stats WHERE session_id = ? AND model = 'unknown'`, "sess-all-nf").Scan(&count))
	require.Equal(t, 1, count)
	// Second SyncOnce must not produce a second tombstone.
	syncer.SyncOnce(context.Background())
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM bee_token_stats WHERE session_id = ?`, "sess-all-nf").Scan(&count))
	assert.Equal(t, 1, count)
}

// TestSyncer_UnknownEngine_FallsBack: engine set but not in adapter map → walks fallback chain.
func TestSyncer_UnknownEngine_FallsBack(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "w1", "")
	insertTestExecutionWithEngine(t, db, "w1", "sess-unknown", "obsolete-engine", time.Now().UnixMilli())

	claudeHit := &fakeAdapter{collect: func(_ context.Context, _ string) ([]ai.TokenUsage, error) {
		return []ai.TokenUsage{{Model: "sonnet-4", InputTokens: 77}}, nil
	}}
	adapters := map[string]ai.EngineAdapter{
		ai.EngineClaude: claudeHit,
	}
	syncer := tokenstat.NewSyncer(db, tokenStore, adapters, []string{ai.EngineClaude})
	syncer.SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("sess-unknown")
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.EqualValues(t, 77, stats[0].InputTokens)
}

// TestSyncer_UnknownEngine_AllNotFound_Tombstones: unknown engine, all fallbacks also NotFound → tombstone.
func TestSyncer_UnknownEngine_AllNotFound_Tombstones(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "w1", "")
	insertTestExecutionWithEngine(t, db, "w1", "sess-unk-nf", "obsolete-engine", time.Now().UnixMilli())

	notFound := &fakeAdapter{collect: func(_ context.Context, _ string) ([]ai.TokenUsage, error) {
		return nil, ai.ErrSessionDataNotFound
	}}
	adapters := map[string]ai.EngineAdapter{
		ai.EngineClaude: notFound,
	}
	syncer := tokenstat.NewSyncer(db, tokenStore, adapters, []string{ai.EngineClaude})
	syncer.SyncOnce(context.Background())

	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM bee_token_stats WHERE session_id = ? AND model = 'unknown'`, "sess-unk-nf").Scan(&count))
	require.Equal(t, 1, count)
}

// TestSyncer_DoesNotResyncCompleted: session with synced_at > completed_at is skipped.
func TestSyncer_DoesNotResyncCompleted(t *testing.T) {
	db, tokenStore, cleanup := newSyncerTestDB(t)
	defer cleanup()

	insertTestWorker(t, db, "w1", "claude")
	completedAt := time.Now().Add(-1 * time.Hour).UnixMilli()
	insertTestExecutionWithEngine(t, db, "w1", "sess-done", "claude", completedAt)

	// Seed a token stat with synced_at > completed_at so the SQL HAVING clause skips it.
	require.NoError(t, tokenStore.Upsert(model.TokenStats{
		SessionID:   "sess-done",
		AgentType:   "claude",
		Model:       "sonnet-4",
		InputTokens: 42,
		SyncedAt:    time.Now().UnixMilli(), // > completedAt
	}))

	callCount := 0
	adapters := map[string]ai.EngineAdapter{
		ai.EngineClaude: &fakeAdapter{collect: func(_ context.Context, _ string) ([]ai.TokenUsage, error) {
			callCount++
			return []ai.TokenUsage{{Model: "sonnet-4", InputTokens: 999}}, nil
		}},
	}
	syncer := tokenstat.NewSyncer(db, tokenStore, adapters, ai.AllEngines())
	syncer.SyncOnce(context.Background())

	assert.Equal(t, 0, callCount, "adapter should not be called for already-synced session")
	// Original value must be unchanged.
	stats, err := tokenStore.GetBySessionID("sess-done")
	require.NoError(t, err)
	require.Len(t, stats, 1) // guard: stats[0] accessed below
	assert.EqualValues(t, 42, stats[0].InputTokens)
}
