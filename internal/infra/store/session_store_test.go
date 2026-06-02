package store_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func setupSessionDB(t *testing.T) (*sql.DB, *store.SessionStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, store.NewSessionStore(db)
}

func TestSessionStore_GetSessionContext_MissReturnsEmpty(t *testing.T) {
	_, ss := setupSessionDB(t)
	got, engine, err := ss.GetSessionContext(context.Background(), "feishu:c:u", store.BeeAgentID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string on miss, got %q", got)
	}
	if engine != "" {
		t.Errorf("expected empty engine on miss, got %q", engine)
	}
}

func TestSessionStore_UpsertAndGet(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	if err := ss.UpsertSessionContext(ctx, "feishu:c:u", store.BeeAgentID, "sess-abc", "claude"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, engine, err := ss.GetSessionContext(ctx, "feishu:c:u", store.BeeAgentID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "sess-abc" {
		t.Errorf("expected sess-abc, got %q", got)
	}
	if engine != "claude" {
		t.Errorf("expected claude, got %q", engine)
	}
}

func TestSessionStore_Upsert_Overwrites(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "k", store.BeeAgentID, "old", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "k", store.BeeAgentID, "new", "claude") //nolint:errcheck

	got, engine, _ := ss.GetSessionContext(ctx, "k", store.BeeAgentID)
	if got != "new" {
		t.Errorf("expected new, got %q", got)
	}
	if engine != "claude" {
		t.Errorf("expected claude, got %q", engine)
	}
}

func TestSessionStore_AgentsAreIsolated(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "k", store.BeeAgentID, "bee-sess", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "k", "worker-1", "worker-sess", "claude")    //nolint:errcheck

	beeSess, _, _ := ss.GetSessionContext(ctx, "k", store.BeeAgentID)
	workerSess, _, _ := ss.GetSessionContext(ctx, "k", "worker-1")
	if beeSess != "bee-sess" {
		t.Errorf("bee: expected bee-sess, got %q", beeSess)
	}
	if workerSess != "worker-sess" {
		t.Errorf("worker: expected worker-sess, got %q", workerSess)
	}
}

func TestSessionStore_ClearSessionContexts(t *testing.T) {
	db, ss := setupSessionDB(t)
	ctx := context.Background()
	ws := store.NewWorkerStore(db)

	// worker-explicit: engine explicitly set to "claude" in bee_workers.
	wExplicit, err := ws.Create(model.Worker{Name: "explicit", WorkDir: t.TempDir(), Engine: "claude"})
	if err != nil {
		t.Fatalf("create worker explicit: %v", err)
	}
	// worker-fallback: no engine set in bee_workers → falls back to beeEngine.
	wFallback, err := ws.Create(model.Worker{Name: "fallback", WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("create worker fallback: %v", err)
	}

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

	if err := ss.ClearSessionContexts(ctx, "k", "claude"); err != nil {
		t.Fatalf("clear: %v", err)
	}

	// bee/claude → cleared.
	beeClaude, _ := ss.GetSessionContextForEngine(ctx, "k", store.BeeAgentID, "claude")
	if beeClaude != "" {
		t.Errorf("bee/claude should be cleared, got %q", beeClaude)
	}
	// bee/codex → retained.
	beeCodex, _ := ss.GetSessionContextForEngine(ctx, "k", store.BeeAgentID, "codex")
	if beeCodex != "bee-codex" {
		t.Errorf("bee/codex should be retained, got %q", beeCodex)
	}
	// worker-explicit/claude → cleared (engine matches bee_workers.engine).
	wexClaude, _ := ss.GetSessionContextForEngine(ctx, "k", wExplicit.ID, "claude")
	if wexClaude != "" {
		t.Errorf("worker-explicit/claude should be cleared, got %q", wexClaude)
	}
	// worker-explicit/codex → retained (engine mismatch).
	wexCodex, _ := ss.GetSessionContextForEngine(ctx, "k", wExplicit.ID, "codex")
	if wexCodex != "wex-codex" {
		t.Errorf("worker-explicit/codex should be retained, got %q", wexCodex)
	}
	// worker-fallback/claude → cleared (engine="" falls back to beeEngine "claude").
	wfbClaude, _ := ss.GetSessionContextForEngine(ctx, "k", wFallback.ID, "claude")
	if wfbClaude != "" {
		t.Errorf("worker-fallback/claude should be cleared, got %q", wfbClaude)
	}
	// ghost worker → all records cleared (orphan cleanup).
	ghostClaude, _ := ss.GetSessionContextForEngine(ctx, "k", "ghost-id", "claude")
	ghostCodex, _ := ss.GetSessionContextForEngine(ctx, "k", "ghost-id", "codex")
	if ghostClaude != "" {
		t.Errorf("ghost/claude should be cleared, got %q", ghostClaude)
	}
	if ghostCodex != "" {
		t.Errorf("ghost/codex should be cleared, got %q", ghostCodex)
	}
	// unrelated session → untouched.
	otherSess, _, _ := ss.GetSessionContext(ctx, "other", store.BeeAgentID)
	if otherSess != "other-sess" {
		t.Errorf("other session must not be cleared, got %q", otherSess)
	}
}

func TestSessionStore_ListSessionContexts_Empty(t *testing.T) {
	_, ss := setupSessionDB(t)
	got, err := ss.ListSessionContexts(context.Background(), "no-such-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestSessionStore_ListSessionContexts_BeeAndWorker(t *testing.T) {
	db, ss := setupSessionDB(t)
	ctx := context.Background()

	ws := store.NewWorkerStore(db)
	w, err := ws.Create(model.Worker{Name: "TianTian", WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	ss.UpsertSessionContext(ctx, "sk", store.BeeAgentID, "bee-sid", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "sk", w.ID, "worker-sid", "claude")          //nolint:errcheck

	got, err := ss.ListSessionContexts(ctx, "sk")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(got), got)
	}

	byAgent := make(map[string]store.SessionAgent)
	for _, a := range got {
		byAgent[a.AgentID] = a
	}

	bee := byAgent[store.BeeAgentID]
	if bee.AgentType != "bee" || bee.Name != "bee" || bee.Engine != "claude" {
		t.Errorf("bee entry: got type=%q name=%q engine=%q", bee.AgentType, bee.Name, bee.Engine)
	}

	wkr := byAgent[w.ID]
	if wkr.AgentType != "worker" || wkr.Name != "TianTian" || wkr.Engine != "claude" {
		t.Errorf("worker entry: got type=%q name=%q engine=%q", wkr.AgentType, wkr.Name, wkr.Engine)
	}
}

func TestSessionStore_ListSessionContexts_DeletedWorker(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "sk", "ghost-worker-id", "sid", "claude") //nolint:errcheck

	got, err := ss.ListSessionContexts(ctx, "sk")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].Name != "(deleted)" {
		t.Errorf("expected (deleted), got %q", got[0].Name)
	}
	if got[0].AgentType != "worker" {
		t.Errorf("expected type=worker, got %q", got[0].AgentType)
	}
	if got[0].Engine != "claude" {
		t.Errorf("expected engine=claude, got %q", got[0].Engine)
	}
}

func TestSessionStore_ListSessionContexts_MultipleEngines(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "sk", "worker-1", "claude-sid", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "sk", "worker-1", "codex-sid", "codex")   //nolint:errcheck

	got, err := ss.ListSessionContexts(ctx, "sk")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}

	engines := map[string]bool{}
	for _, entry := range got {
		engines[entry.Engine] = true
	}
	if !engines["claude"] || !engines["codex"] {
		t.Fatalf("expected both claude and codex entries, got %+v", got)
	}
}

func TestSessionStore_ListActiveSessionContexts(t *testing.T) {
	db, ss := setupSessionDB(t)
	ctx := context.Background()
	ws := store.NewWorkerStore(db)

	wExplicitClaude, err := ws.Create(model.Worker{Name: "explicit-claude", WorkDir: t.TempDir(), Engine: "claude"})
	if err != nil {
		t.Fatalf("create wExplicitClaude: %v", err)
	}
	wExplicitCodex, err := ws.Create(model.Worker{Name: "explicit-codex", WorkDir: t.TempDir(), Engine: "codex"})
	if err != nil {
		t.Fatalf("create wExplicitCodex: %v", err)
	}
	wFallback, err := ws.Create(model.Worker{Name: "fallback", WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("create wFallback: %v", err)
	}

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
	if err != nil {
		t.Fatalf("list active: %v", err)
	}

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
		if _, ok := active[k]; !ok {
			t.Errorf("expected active entry for %+v, not returned", k)
		}
	}

	wantInactive := []key{
		{store.BeeAgentID, "codex"},
		{wExplicitClaude.ID, "codex"},
		{wExplicitCodex.ID, "claude"},
	}
	for _, k := range wantInactive {
		if _, ok := active[k]; ok {
			t.Errorf("expected inactive entry %+v to be filtered out", k)
		}
	}

	for _, a := range got {
		switch a.AgentID {
		case store.BeeAgentID:
			if a.AgentType != "bee" || a.Name != "bee" {
				t.Errorf("bee row: got type=%q name=%q", a.AgentType, a.Name)
			}
		case "ghost-id":
			if a.AgentType != "worker" || a.Name != "(deleted)" {
				t.Errorf("ghost row: got type=%q name=%q", a.AgentType, a.Name)
			}
		default:
			if a.AgentType != "worker" || a.Name == "" {
				t.Errorf("worker row %s: got type=%q name=%q", a.AgentID, a.AgentType, a.Name)
			}
		}
	}
}

func TestSessionStore_DeleteSessionContextForEngine_Basic(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "sk", "worker-1", "w1-claude", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "sk", "worker-1", "w1-codex", "codex")   //nolint:errcheck

	if _, err := ss.DeleteSessionContextForEngine(ctx, "sk", "worker-1", "codex"); err != nil {
		t.Fatalf("delete by engine: %v", err)
	}

	claude, _ := ss.GetSessionContextForEngine(ctx, "sk", "worker-1", "claude")
	codex, _ := ss.GetSessionContextForEngine(ctx, "sk", "worker-1", "codex")
	if claude != "w1-claude" {
		t.Errorf("expected claude context preserved, got %q", claude)
	}
	if codex != "" {
		t.Errorf("expected codex context cleared, got %q", codex)
	}
}

func TestSessionStore_GetSessionContextForEngine_EngineMismatch(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	// Store a claude session
	if err := ss.UpsertSessionContext(ctx, "sk", store.BeeAgentID, "claude-sid", "claude"); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Switching to codex must not reuse the claude session
	got, err := ss.GetSessionContextForEngine(ctx, "sk", store.BeeAgentID, "codex")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty session for codex after claude stored, got %q", got)
	}
}

func TestSessionStore_DifferentEnginesCoexist(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "sk", store.BeeAgentID, "claude-sid", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "sk", store.BeeAgentID, "codex-sid", "codex")   //nolint:errcheck

	claude, err := ss.GetSessionContextForEngine(ctx, "sk", store.BeeAgentID, "claude")
	if err != nil {
		t.Fatalf("get claude: %v", err)
	}
	codex, err := ss.GetSessionContextForEngine(ctx, "sk", store.BeeAgentID, "codex")
	if err != nil {
		t.Fatalf("get codex: %v", err)
	}
	if claude != "claude-sid" {
		t.Errorf("expected claude-sid, got %q", claude)
	}
	if codex != "codex-sid" {
		t.Errorf("expected codex-sid, got %q", codex)
	}
}
