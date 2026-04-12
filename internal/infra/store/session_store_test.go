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
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "k", store.BeeAgentID, "bee-sess", "claude")  //nolint:errcheck
	ss.UpsertSessionContext(ctx, "k", "worker-1", "w1-sess", "claude")         //nolint:errcheck
	ss.UpsertSessionContext(ctx, "other", store.BeeAgentID, "other", "claude") //nolint:errcheck

	if err := ss.ClearSessionContexts(ctx, "k"); err != nil {
		t.Fatalf("clear: %v", err)
	}

	beeSess, _, _ := ss.GetSessionContext(ctx, "k", store.BeeAgentID)
	w1Sess, _, _ := ss.GetSessionContext(ctx, "k", "worker-1")
	otherSess, _, _ := ss.GetSessionContext(ctx, "other", store.BeeAgentID)

	if beeSess != "" {
		t.Errorf("expected bee session cleared, got %q", beeSess)
	}
	if w1Sess != "" {
		t.Errorf("expected worker session cleared, got %q", w1Sess)
	}
	if otherSess != "other" {
		t.Errorf("other key must not be cleared, got %q", otherSess)
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

func TestSessionStore_DeleteWorkerSessionContext_Basic(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "sk", store.BeeAgentID, "bee-sid", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "sk", "worker-1", "w1-claude", "claude")     //nolint:errcheck
	ss.UpsertSessionContext(ctx, "sk", "worker-1", "w1-codex", "codex")       //nolint:errcheck

	if err := ss.DeleteWorkerSessionContext(ctx, "sk", "worker-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	w1Claude, _ := ss.GetSessionContextForEngine(ctx, "sk", "worker-1", "claude")
	w1Codex, _ := ss.GetSessionContextForEngine(ctx, "sk", "worker-1", "codex")
	if w1Claude != "" || w1Codex != "" {
		t.Errorf("expected worker-1 cleared across engines, got claude=%q codex=%q", w1Claude, w1Codex)
	}
	bee, _ := ss.GetSessionContextForEngine(ctx, "sk", store.BeeAgentID, "claude")
	if bee != "bee-sid" {
		t.Errorf("expected bee unaffected, got %q", bee)
	}
}

func TestSessionStore_DeleteWorkerSessionContext_Idempotent(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	if err := ss.DeleteWorkerSessionContext(ctx, "sk", "nobody"); err != nil {
		t.Errorf("expected no error on missing row, got %v", err)
	}
}

func TestSessionStore_DeleteSessionContextForEngine_Basic(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "sk", "worker-1", "w1-claude", "claude") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "sk", "worker-1", "w1-codex", "codex")   //nolint:errcheck

	if err := ss.DeleteSessionContextForEngine(ctx, "sk", "worker-1", "codex"); err != nil {
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
