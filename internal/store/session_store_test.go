package store_test

import (
	"context"
	"database/sql"
	"testing"

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
