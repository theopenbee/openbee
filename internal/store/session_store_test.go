package store_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/store"
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
	got, err := ss.GetSessionContext(context.Background(), "feishu:c:u", store.BeeAgentID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string on miss, got %q", got)
	}
}

func TestSessionStore_UpsertAndGet(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	if err := ss.UpsertSessionContext(ctx, "feishu:c:u", store.BeeAgentID, "sess-abc"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := ss.GetSessionContext(ctx, "feishu:c:u", store.BeeAgentID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "sess-abc" {
		t.Errorf("expected sess-abc, got %q", got)
	}
}

func TestSessionStore_Upsert_Overwrites(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "k", store.BeeAgentID, "old") //nolint:errcheck
	ss.UpsertSessionContext(ctx, "k", store.BeeAgentID, "new") //nolint:errcheck

	got, _ := ss.GetSessionContext(ctx, "k", store.BeeAgentID)
	if got != "new" {
		t.Errorf("expected new, got %q", got)
	}
}

func TestSessionStore_AgentsAreIsolated(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "k", store.BeeAgentID, "bee-sess")   //nolint:errcheck
	ss.UpsertSessionContext(ctx, "k", "worker-1", "worker-sess")       //nolint:errcheck

	beeSess, _ := ss.GetSessionContext(ctx, "k", store.BeeAgentID)
	workerSess, _ := ss.GetSessionContext(ctx, "k", "worker-1")
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

	ss.UpsertSessionContext(ctx, "k", store.BeeAgentID, "bee-sess")  //nolint:errcheck
	ss.UpsertSessionContext(ctx, "k", "worker-1", "w1-sess")          //nolint:errcheck
	ss.UpsertSessionContext(ctx, "other", store.BeeAgentID, "other")  //nolint:errcheck

	if err := ss.ClearSessionContexts(ctx, "k"); err != nil {
		t.Fatalf("clear: %v", err)
	}

	beeSess, _ := ss.GetSessionContext(ctx, "k", store.BeeAgentID)
	w1Sess, _ := ss.GetSessionContext(ctx, "k", "worker-1")
	otherSess, _ := ss.GetSessionContext(ctx, "other", store.BeeAgentID)

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
	w, err := ws.Create(model.Worker{Name: "天天", WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	ss.UpsertSessionContext(ctx, "sk", store.BeeAgentID, "bee-sid")   //nolint:errcheck
	ss.UpsertSessionContext(ctx, "sk", w.ID, "worker-sid")            //nolint:errcheck

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
	if bee.AgentType != "bee" || bee.Name != "bee" {
		t.Errorf("bee entry: got type=%q name=%q", bee.AgentType, bee.Name)
	}

	wkr := byAgent[w.ID]
	if wkr.AgentType != "worker" || wkr.Name != "天天" {
		t.Errorf("worker entry: got type=%q name=%q", wkr.AgentType, wkr.Name)
	}
}

func TestSessionStore_ListSessionContexts_DeletedWorker(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "sk", "ghost-worker-id", "sid") //nolint:errcheck

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
}

func TestSessionStore_DeleteWorkerSessionContext_Basic(t *testing.T) {
	_, ss := setupSessionDB(t)
	ctx := context.Background()

	ss.UpsertSessionContext(ctx, "sk", store.BeeAgentID, "bee-sid")   //nolint:errcheck
	ss.UpsertSessionContext(ctx, "sk", "worker-1", "w1-sid")          //nolint:errcheck

	if err := ss.DeleteWorkerSessionContext(ctx, "sk", "worker-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	w1, _ := ss.GetSessionContext(ctx, "sk", "worker-1")
	if w1 != "" {
		t.Errorf("expected worker-1 cleared, got %q", w1)
	}
	bee, _ := ss.GetSessionContext(ctx, "sk", store.BeeAgentID)
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
