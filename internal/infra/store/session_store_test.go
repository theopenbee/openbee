package store_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func setupSessionDB(t *testing.T) (*sql.DB, *store.SessionStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db, store.NewSessionStore(db)
}

// TestSessionStore_GetSessionContext merges the former MissReturnsEmpty,
// UpsertAndGet, and GetSessionContextForEngine_EngineMismatch tests: all three
// upsert (optionally) then read back a session, differing only in which
// lookup method is used and what they expect to find.
func TestSessionStore_GetSessionContext(t *testing.T) {
	tests := []struct {
		name       string
		upsert     bool
		forEngine  string // non-empty uses GetSessionContextForEngine instead of GetSessionContext
		wantSessID string
	}{
		{name: "MissReturnsEmpty"},
		{name: "UpsertAndGet", upsert: true, wantSessID: "sess-abc"},
		{name: "EngineMismatch", upsert: true, forEngine: "codex"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ss := setupSessionDB(t)
			ctx := context.Background()
			if tt.upsert {
				require.NoError(t, ss.UpsertSessionContext(ctx, "sk", store.BeeAgentID, "sess-abc", "claude"))
			}
			if tt.forEngine != "" {
				got, err := ss.GetSessionContextForEngine(ctx, "sk", store.BeeAgentID, tt.forEngine)
				require.NoError(t, err)
				assert.Equal(t, tt.wantSessID, got)
				return
			}
			got, engine, err := ss.GetSessionContext(ctx, "sk", store.BeeAgentID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantSessID, got)
			wantEngine := ""
			if tt.upsert {
				wantEngine = "claude"
			}
			assert.Equal(t, wantEngine, engine)
		})
	}
}

func TestSessionStore_Upsert_Overwrites(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "k", store.BeeAgentID, "old", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "k", store.BeeAgentID, "new", "claude") //nolint:errcheck

	got, engine, _ := ss.GetSessionContext(ctx, "k", store.BeeAgentID)
	assert.Equal(t, "new", got)
	assert.Equal(t, "claude", engine)
}

func TestSessionStore_AgentsAreIsolated(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "k", store.BeeAgentID, "bee-sess", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "k", "worker-1", "worker-sess", "claude")    //nolint:errcheck

	beeSess, _, _ := ss.GetSessionContext(ctx, "k", store.BeeAgentID)
	workerSess, _, _ := ss.GetSessionContext(ctx, "k", "worker-1")
	assert.Equal(t, "bee-sess", beeSess, "bee")
	assert.Equal(t, "worker-sess", workerSess, "worker")
}

func TestSessionStore_ClearSessionContexts(t *testing.T) {
	db, ss := setupSessionDB(t)
	ctx := context.Background()
	ws := store.NewWorkerStore(db)

	// worker-explicit: engine explicitly set to "claude" in bee_workers.
	wExplicit, err := ws.Create(model.Worker{Name: "explicit", WorkDir: t.TempDir(), Engine: "claude"})
	require.NoError(t, err, "create worker explicit")
	// worker-fallback: no engine set in bee_workers → falls back to beeEngine.
	wFallback, err := ws.Create(model.Worker{Name: "fallback", WorkDir: t.TempDir()})
	require.NoError(t, err, "create worker fallback")

	// bee: two engines.
	ss.UpsertSessionContext(ctx, "k", store.BeeAgentID, "bee-claude", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "k", store.BeeAgentID, "bee-codex", "codex")   //nolint:errcheck
	// worker-explicit: two engines.
	ss.UpsertSessionContext(ctx, "k", wExplicit.ID, "wex-claude", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "k", wExplicit.ID, "wex-codex", "codex")   //nolint:errcheck
	// worker-fallback (engine="" in bee_workers): recorded under claude via fallback.
	ss.UpsertSessionContext(ctx, "k", wFallback.ID, "wfb-claude", "claude") //nolint:errcheck
	// deleted worker: orphan data for both engines.
	ss.UpsertSessionContext(ctx, "k", "ghost-id", "ghost-claude", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "k", "ghost-id", "ghost-codex", "codex")   //nolint:errcheck
	// unrelated session: must survive.
	ss.UpsertSessionContext(ctx, "other", store.BeeAgentID, "other-sess", "claude") //nolint:errcheck

	require.NoError(t, ss.ClearSessionContexts(ctx, "k", "claude"))

	// bee/claude → cleared.
	beeClaude, _ := ss.GetSessionContextForEngine(ctx, "k", store.BeeAgentID, "claude")
	assert.Equal(t, "", beeClaude, "bee/claude should be cleared")
	// bee/codex → retained.
	beeCodex, _ := ss.GetSessionContextForEngine(ctx, "k", store.BeeAgentID, "codex")
	assert.Equal(t, "bee-codex", beeCodex, "bee/codex should be retained")
	// worker-explicit/claude → cleared (engine matches bee_workers.engine).
	wexClaude, _ := ss.GetSessionContextForEngine(ctx, "k", wExplicit.ID, "claude")
	assert.Equal(t, "", wexClaude, "worker-explicit/claude should be cleared")
	// worker-explicit/codex → retained (engine mismatch).
	wexCodex, _ := ss.GetSessionContextForEngine(ctx, "k", wExplicit.ID, "codex")
	assert.Equal(t, "wex-codex", wexCodex, "worker-explicit/codex should be retained")
	// worker-fallback/claude → cleared (engine="" falls back to beeEngine "claude").
	wfbClaude, _ := ss.GetSessionContextForEngine(ctx, "k", wFallback.ID, "claude")
	assert.Equal(t, "", wfbClaude, "worker-fallback/claude should be cleared")
	// ghost worker → all records cleared (orphan cleanup).
	ghostClaude, _ := ss.GetSessionContextForEngine(ctx, "k", "ghost-id", "claude")
	ghostCodex, _ := ss.GetSessionContextForEngine(ctx, "k", "ghost-id", "codex")
	assert.Equal(t, "", ghostClaude, "ghost/claude should be cleared")
	assert.Equal(t, "", ghostCodex, "ghost/codex should be cleared")
	// unrelated session → untouched.
	otherSess, _, _ := ss.GetSessionContext(ctx, "other", store.BeeAgentID)
	assert.Equal(t, "other-sess", otherSess, "other session must not be cleared")
}

func TestSessionStore_ListSessionContexts_Empty(t *testing.T) {
	_, ss := setupSessionDB(t)
	got, err := ss.ListSessionContexts(context.Background(), "no-such-session")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestSessionStore_ListSessionContexts_BeeAndWorker(t *testing.T) {
	db, ss := setupSessionDB(t)
	ctx := context.Background()

	ws := store.NewWorkerStore(db)
	w, err := ws.Create(model.Worker{Name: "TianTian", WorkDir: t.TempDir()})
	require.NoError(t, err, "create worker")

	ss.UpsertSessionContext(ctx, "sk", store.BeeAgentID, "bee-sid", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "sk", w.ID, "worker-sid", "claude")          //nolint:errcheck

	got, err := ss.ListSessionContexts(ctx, "sk")
	require.NoError(t, err)
	require.Len(t, got, 2)

	byAgent := make(map[string]store.SessionAgent)
	for _, a := range got {
		byAgent[a.AgentID] = a
	}

	bee := byAgent[store.BeeAgentID]
	assert.Equal(t, "bee", bee.AgentType, "bee entry")
	assert.Equal(t, "bee", bee.Name, "bee entry")
	assert.Equal(t, "claude", bee.Engine, "bee entry")

	wkr := byAgent[w.ID]
	assert.Equal(t, "worker", wkr.AgentType, "worker entry")
	assert.Equal(t, "TianTian", wkr.Name, "worker entry")
	assert.Equal(t, "claude", wkr.Engine, "worker entry")
}

func TestSessionStore_ListSessionContexts_DeletedWorker(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "sk", "ghost-worker-id", "sid", "claude") //nolint:errcheck

	got, err := ss.ListSessionContexts(ctx, "sk")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "(deleted)", got[0].Name)
	assert.Equal(t, "worker", got[0].AgentType)
	assert.Equal(t, "claude", got[0].Engine)
}

func TestSessionStore_ListSessionContexts_MultipleEngines(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "sk", "worker-1", "claude-sid", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "sk", "worker-1", "codex-sid", "codex")   //nolint:errcheck

	got, err := ss.ListSessionContexts(ctx, "sk")
	require.NoError(t, err)
	require.Len(t, got, 2)

	engines := map[string]bool{}
	for _, entry := range got {
		engines[entry.Engine] = true
	}
	require.True(t, engines["claude"], "expected claude entry, got %+v", got)
	require.True(t, engines["codex"], "expected codex entry, got %+v", got)
}

func TestSessionStore_ListActiveSessionContexts(t *testing.T) {
	db, ss := setupSessionDB(t)
	ctx := context.Background()
	ws := store.NewWorkerStore(db)

	wExplicitClaude, err := ws.Create(model.Worker{Name: "explicit-claude", WorkDir: t.TempDir(), Engine: "claude"})
	require.NoError(t, err, "create wExplicitClaude")
	wExplicitCodex, err := ws.Create(model.Worker{Name: "explicit-codex", WorkDir: t.TempDir(), Engine: "codex"})
	require.NoError(t, err, "create wExplicitCodex")
	wFallback, err := ws.Create(model.Worker{Name: "fallback", WorkDir: t.TempDir()})
	require.NoError(t, err, "create wFallback")

	// bee under two engines.
	ss.UpsertSessionContext(ctx, "k", store.BeeAgentID, "bee-claude", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "k", store.BeeAgentID, "bee-codex", "codex")   //nolint:errcheck
	// explicit-claude worker: claude row matches; codex row is stale (engine mismatch).
	ss.UpsertSessionContext(ctx, "k", wExplicitClaude.ID, "ec-claude", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "k", wExplicitClaude.ID, "ec-codex", "codex")   //nolint:errcheck
	// explicit-codex worker: codex row matches; claude row is stale.
	ss.UpsertSessionContext(ctx, "k", wExplicitCodex.ID, "ex-claude", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "k", wExplicitCodex.ID, "ex-codex", "codex")   //nolint:errcheck
	// fallback worker (engine=""): only the beeEngine row is active.
	ss.UpsertSessionContext(ctx, "k", wFallback.ID, "fb-claude", "claude") //nolint:errcheck
	// orphan worker (no bee_workers row): all rows are active.
	ss.UpsertSessionContext(ctx, "k", "ghost-id", "ghost-claude", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "k", "ghost-id", "ghost-codex", "codex")   //nolint:errcheck
	// unrelated session must not leak.
	ss.UpsertSessionContext(ctx, "other", store.BeeAgentID, "other-sess", "claude") //nolint:errcheck

	got, err := ss.ListActiveSessionContexts(ctx, "k", "claude")
	require.NoError(t, err, "list active")

	type key struct{ agent, engine string }
	active := make(map[key]store.SessionAgent)
	for _, a := range got {
		active[key{a.AgentID, a.Engine}] = a
	}

	wantActive := []key{
		{store.BeeAgentID, "claude"},
		{wExplicitClaude.ID, "claude"},
		{wExplicitCodex.ID, "codex"},
		{wFallback.ID, "claude"},
		{"ghost-id", "claude"},
		{"ghost-id", "codex"},
	}
	for _, k := range wantActive {
		_, ok := active[k]
		assert.True(t, ok, "expected active entry for %+v, not returned", k)
	}

	wantInactive := []key{
		{store.BeeAgentID, "codex"},
		{wExplicitClaude.ID, "codex"},
		{wExplicitCodex.ID, "claude"},
	}
	for _, k := range wantInactive {
		_, ok := active[k]
		assert.False(t, ok, "expected inactive entry %+v to be filtered out", k)
	}

	for _, a := range got {
		switch a.AgentID {
		case store.BeeAgentID:
			assert.Equal(t, "bee", a.AgentType, "bee row")
			assert.Equal(t, "bee", a.Name, "bee row")
		case "ghost-id":
			assert.Equal(t, "worker", a.AgentType, "ghost row")
			assert.Equal(t, "(deleted)", a.Name, "ghost row")
		default:
			assert.Equal(t, "worker", a.AgentType, "worker row %s", a.AgentID)
			assert.NotEmpty(t, a.Name, "worker row %s", a.AgentID)
		}
	}
}

func TestSessionStore_DeleteSessionContextForEngine_Basic(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "sk", "worker-1", "w1-claude", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "sk", "worker-1", "w1-codex", "codex")   //nolint:errcheck

	_, err := ss.DeleteSessionContextForEngine(ctx, "sk", "worker-1", "codex")
	require.NoError(t, err, "delete by engine")

	claude, _ := ss.GetSessionContextForEngine(ctx, "sk", "worker-1", "claude")
	codex, _ := ss.GetSessionContextForEngine(ctx, "sk", "worker-1", "codex")
	assert.Equal(t, "w1-claude", claude, "expected claude context preserved")
	assert.Equal(t, "", codex, "expected codex context cleared")
}

func TestSessionStore_DifferentEnginesCoexist(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "sk", store.BeeAgentID, "claude-sid", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "sk", store.BeeAgentID, "codex-sid", "codex")   //nolint:errcheck

	claude, err := ss.GetSessionContextForEngine(ctx, "sk", store.BeeAgentID, "claude")
	require.NoError(t, err, "get claude")
	codex, err := ss.GetSessionContextForEngine(ctx, "sk", store.BeeAgentID, "codex")
	require.NoError(t, err, "get codex")
	assert.Equal(t, "claude-sid", claude)
	assert.Equal(t, "codex-sid", codex)
}

func TestSessionStore_ReassignSessionKey(t *testing.T) {
	db, ss := setupSessionDB(t)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO bee_platform_messages (id, session_key, platform, content, received_at, created_at, updated_at)
		 VALUES ('in-old', 'local:old', 'local', 'hi', 1, 1, 1),
		        ('in-other', 'feishu:chat:user', 'feishu', 'hi', 1, 1, 1)`,
		`INSERT INTO bee_outbound_messages (id, session_key, platform, content, status, sent_at, created_at)
		 VALUES ('out-old', 'local:old', 'local', 'hello', 'sent', 2, 2)`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}
	require.NoError(t, ss.UpsertSessionContext(ctx, "local:old", store.BeeAgentID, "old-bee", "claude"))
	require.NoError(t, ss.UpsertSessionContext(ctx, "local:old", "w1", "old-worker", "claude"))
	require.NoError(t, ss.UpsertSessionContext(ctx, "local:new", store.BeeAgentID, "new-bee", "claude"))

	require.NoError(t, ss.ReassignSessionKey(ctx, "local:old", "local:new"))

	sessionKeyOf := func(query, id string) string {
		var key string
		require.NoError(t, db.QueryRow(query, id).Scan(&key))
		return key
	}
	assert.Equal(t, "local:new", sessionKeyOf(`SELECT session_key FROM bee_platform_messages WHERE id = ?`, "in-old"))
	assert.Equal(t, "feishu:chat:user", sessionKeyOf(`SELECT session_key FROM bee_platform_messages WHERE id = ?`, "in-other"))
	assert.Equal(t, "local:new", sessionKeyOf(`SELECT session_key FROM bee_outbound_messages WHERE id = ?`, "out-old"))

	worker, err := ss.GetSessionContextForEngine(ctx, "local:new", "w1", "claude")
	require.NoError(t, err)
	assert.Equal(t, "old-worker", worker)
	bee, err := ss.GetSessionContextForEngine(ctx, "local:new", store.BeeAgentID, "claude")
	require.NoError(t, err)
	assert.Equal(t, "new-bee", bee)
}
