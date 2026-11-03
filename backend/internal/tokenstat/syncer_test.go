package tokenstat_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/aitest"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/tokenstat"
)

func newSyncerTestDB(t *testing.T) (*sql.DB, *store.TokenStatsStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db, store.NewTokenStatsStore(db)
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

func usageFor(found map[string][]ai.TokenUsage) func(context.Context, []string) (map[string][]ai.TokenUsage, error) {
	return func(_ context.Context, ids []string) (map[string][]ai.TokenUsage, error) {
		out := make(map[string][]ai.TokenUsage)
		for _, id := range ids {
			if u, ok := found[id]; ok {
				out[id] = u
			}
		}
		return out, nil
	}
}

func tombstoneCount(t *testing.T, db *sql.DB, sessionID string) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM bee_token_stats WHERE session_id = ? AND model = ?`, sessionID, store.TombstoneModel).Scan(&count))
	return count
}

func rowCount(t *testing.T, db *sql.DB, sessionID string) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM bee_token_stats WHERE session_id = ?`, sessionID).Scan(&count))
	return count
}

func TestSyncer_Direct_KnownEngine(t *testing.T) {
	db, tokenStore := newSyncerTestDB(t)
	insertTestWorker(t, db, "w1", "claude")
	insertTestExecutionWithEngine(t, db, "w1", "sess-1", "claude", time.Now().UnixMilli())

	claude := &aitest.Engine{UsageFunc: usageFor(map[string][]ai.TokenUsage{
		"sess-1": {{Model: "sonnet-4", InputTokens: 100, OutputTokens: 50}},
	})}
	tokenstat.NewSyncer(db, tokenStore, aitest.NewSource(map[string]ai.EngineAdapter{"claude": claude})).SyncOnce(context.Background())

	stats, err := tokenStore.GetBySessionID("sess-1")
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, "sonnet-4", stats[0].Model)
	assert.EqualValues(t, 100, stats[0].InputTokens)
	assert.Equal(t, "claude", stats[0].AgentType)
}

func TestSyncer_Direct_KnownEngine_Tombstones(t *testing.T) {
	cases := []struct {
		name          string
		usage         map[string][]ai.TokenUsage
		err           error
		wantTombstone bool
	}{
		{name: "Missing_Tombstones", usage: map[string][]ai.TokenUsage{}, wantTombstone: true},
		{name: "Empty_Tombstones", usage: map[string][]ai.TokenUsage{"sess": {}}, wantTombstone: true},
		{name: "HardError_NoTombstone", err: context.DeadlineExceeded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, tokenStore := newSyncerTestDB(t)
			insertTestWorker(t, db, "w1", "claude")
			insertTestExecutionWithEngine(t, db, "w1", "sess", "claude", time.Now().UnixMilli())

			claude := &aitest.Engine{UsageFunc: func(context.Context, []string) (map[string][]ai.TokenUsage, error) {
				return tc.usage, tc.err
			}}
			tokenstat.NewSyncer(db, tokenStore, aitest.NewSource(map[string]ai.EngineAdapter{"claude": claude})).SyncOnce(context.Background())

			if tc.wantTombstone {
				require.Equal(t, 1, tombstoneCount(t, db, "sess"))
				stats, err := tokenStore.GetBySessionID("sess")
				require.NoError(t, err)
				assert.Empty(t, stats)
			} else {
				assert.Zero(t, rowCount(t, db, "sess"))
			}
		})
	}
}

func TestSyncer_BatchesPerEngine(t *testing.T) {
	db, tokenStore := newSyncerTestDB(t)
	insertTestWorker(t, db, "w1", "")
	now := time.Now().UnixMilli()
	for _, s := range []struct{ id, engine string }{
		{"c1", "claude"}, {"c2", "claude"}, {"c3", "claude"}, {"p1", "pi"}, {"p2", "pi"},
	} {
		insertTestExecutionWithEngine(t, db, "w1", s.id, s.engine, now)
	}

	claude := &aitest.Engine{UsageFunc: usageFor(map[string][]ai.TokenUsage{
		"c1": {{Model: "sonnet-4", InputTokens: 1}},
		"c3": {{Model: "opus-4", InputTokens: 3}},
	})}
	pi := &aitest.Engine{UsageFunc: usageFor(map[string][]ai.TokenUsage{
		"p1": {{Model: "deepseek", InputTokens: 10}},
		"p2": {{Model: "deepseek", InputTokens: 20}},
	})}
	src := aitest.NewSource(map[string]ai.EngineAdapter{"claude": claude, "pi": pi})
	tokenstat.NewSyncer(db, tokenStore, src).SyncOnce(context.Background())

	require.Len(t, claude.UsageCalls(), 1)
	assert.ElementsMatch(t, []string{"c1", "c2", "c3"}, claude.UsageCalls()[0])
	require.Len(t, pi.UsageCalls(), 1)
	assert.ElementsMatch(t, []string{"p1", "p2"}, pi.UsageCalls()[0])

	for id, want := range map[string]string{"c1": "claude", "c3": "claude", "p1": "pi", "p2": "pi"} {
		stats, err := tokenStore.GetBySessionID(id)
		require.NoError(t, err)
		require.Len(t, stats, 1, id)
		assert.Equal(t, want, stats[0].AgentType, id)
	}
	assert.Equal(t, 1, tombstoneCount(t, db, "c2"))
}

func TestSyncer_SkipsUninstalledEngine(t *testing.T) {
	db, tokenStore := newSyncerTestDB(t)
	insertTestWorker(t, db, "w1", "")
	now := time.Now().UnixMilli()
	insertTestExecutionWithEngine(t, db, "w1", "c1", "codex", now)
	insertTestExecutionWithEngine(t, db, "w1", "p1", "pi", now)

	pi := &aitest.Engine{UsageFunc: usageFor(map[string][]ai.TokenUsage{"p1": {{Model: "deepseek", InputTokens: 1}}})}
	src := aitest.NewSource(map[string]ai.EngineAdapter{"pi": pi})
	syncer := tokenstat.NewSyncer(db, tokenStore, src)
	syncer.SyncOnce(context.Background())
	syncer.SyncOnce(context.Background())

	assert.Equal(t, [][]string{{"p1"}}, pi.UsageCalls())
	assert.Zero(t, rowCount(t, db, "c1"))

	codex := &aitest.Engine{UsageFunc: usageFor(map[string][]ai.TokenUsage{"c1": {{Model: "o3", InputTokens: 7}}})}
	src.Add("codex", map[string]string{"en": "Codex"}, codex)
	syncer.SyncOnce(context.Background())

	assert.Equal(t, [][]string{{"c1"}}, codex.UsageCalls())
	stats, err := tokenStore.GetBySessionID("c1")
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, "codex", stats[0].AgentType)
}

func TestSyncer_LegacyEmptyEngineFallback(t *testing.T) {
	t.Run("WalksEnginesInIDOrder", func(t *testing.T) {
		db, tokenStore := newSyncerTestDB(t)
		insertTestWorker(t, db, "w1", "")
		for _, id := range []string{"l1", "l2", "l3", "l4"} {
			insertTestExecutionWithEngine(t, db, "w1", id, "", time.Now().UnixMilli())
		}

		claude := &aitest.Engine{UsageFunc: usageFor(map[string][]ai.TokenUsage{"l1": {{Model: "sonnet-4", InputTokens: 1}}})}
		codex := &aitest.Engine{UsageFunc: usageFor(map[string][]ai.TokenUsage{"l2": {}})}
		pi := &aitest.Engine{UsageFunc: usageFor(map[string][]ai.TokenUsage{"l3": {{Model: "deepseek", InputTokens: 3}}})}
		src := aitest.NewSource(map[string]ai.EngineAdapter{"pi": pi, "claude": claude, "codex": codex})
		syncer := tokenstat.NewSyncer(db, tokenStore, src)
		syncer.SyncOnce(context.Background())

		require.Len(t, claude.UsageCalls(), 1)
		assert.ElementsMatch(t, []string{"l1", "l2", "l3", "l4"}, claude.UsageCalls()[0])
		require.Len(t, codex.UsageCalls(), 1)
		assert.ElementsMatch(t, []string{"l2", "l3", "l4"}, codex.UsageCalls()[0])
		require.Len(t, pi.UsageCalls(), 1)
		assert.ElementsMatch(t, []string{"l3", "l4"}, pi.UsageCalls()[0])

		for id, want := range map[string]string{"l1": "claude", "l3": "pi"} {
			stats, err := tokenStore.GetBySessionID(id)
			require.NoError(t, err)
			require.Len(t, stats, 1, id)
			assert.Equal(t, want, stats[0].AgentType, id)
		}
		assert.Equal(t, 1, tombstoneCount(t, db, "l2"))
		assert.Equal(t, 1, tombstoneCount(t, db, "l4"))

		syncer.SyncOnce(context.Background())
		assert.Equal(t, 1, rowCount(t, db, "l4"))
		assert.Len(t, claude.UsageCalls(), 1)
	})

	t.Run("ErrorStopsWalk", func(t *testing.T) {
		db, tokenStore := newSyncerTestDB(t)
		insertTestWorker(t, db, "w1", "")
		insertTestExecutionWithEngine(t, db, "w1", "l1", "", time.Now().UnixMilli())

		claude := &aitest.Engine{UsageFunc: func(context.Context, []string) (map[string][]ai.TokenUsage, error) {
			return nil, context.DeadlineExceeded
		}}
		pi := &aitest.Engine{}
		src := aitest.NewSource(map[string]ai.EngineAdapter{"claude": claude, "pi": pi})
		tokenstat.NewSyncer(db, tokenStore, src).SyncOnce(context.Background())

		assert.Empty(t, pi.UsageCalls())
		assert.Zero(t, rowCount(t, db, "l1"))
	})

	t.Run("NoEnginesSkips", func(t *testing.T) {
		db, tokenStore := newSyncerTestDB(t)
		insertTestWorker(t, db, "w1", "")
		insertTestExecutionWithEngine(t, db, "w1", "l1", "", time.Now().UnixMilli())

		tokenstat.NewSyncer(db, tokenStore, aitest.NewSource(nil)).SyncOnce(context.Background())

		assert.Zero(t, rowCount(t, db, "l1"))
	})
}

func TestSyncer_DoesNotResyncCompleted(t *testing.T) {
	db, tokenStore := newSyncerTestDB(t)
	insertTestWorker(t, db, "w1", "claude")
	completedAt := time.Now().Add(-1 * time.Hour).UnixMilli()
	insertTestExecutionWithEngine(t, db, "w1", "sess-done", "claude", completedAt)

	require.NoError(t, tokenStore.Upsert(model.TokenStats{
		SessionID:   "sess-done",
		AgentType:   "claude",
		Model:       "sonnet-4",
		InputTokens: 42,
		SyncedAt:    time.Now().UnixMilli(),
	}))

	claude := &aitest.Engine{UsageFunc: usageFor(map[string][]ai.TokenUsage{"sess-done": {{Model: "sonnet-4", InputTokens: 999}}})}
	tokenstat.NewSyncer(db, tokenStore, aitest.NewSource(map[string]ai.EngineAdapter{"claude": claude})).SyncOnce(context.Background())

	assert.Empty(t, claude.UsageCalls())
	stats, err := tokenStore.GetBySessionID("sess-done")
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.EqualValues(t, 42, stats[0].InputTokens)
}
