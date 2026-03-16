package store_test

import (
	"context"
	"testing"

	"github.com/robobee/core/internal/store"
)

func setupLocalReplyDB(t *testing.T) *store.LocalReplyStore {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store.NewLocalReplyStore(db)
}

func TestLocalReplyStore_CreateAndList(t *testing.T) {
	s := setupLocalReplyDB(t)
	ctx := context.Background()

	if err := s.Create(ctx, "r-1", "local:sess-1", "Hello from bee"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	replies, err := s.ListBySession(ctx, "local:sess-1")
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(replies))
	}
	if replies[0].Content != "Hello from bee" {
		t.Errorf("unexpected content: %q", replies[0].Content)
	}
}

func TestLocalReplyStore_ListBySession_IsolatesSessions(t *testing.T) {
	s := setupLocalReplyDB(t)
	ctx := context.Background()

	s.Create(ctx, "r-1", "local:sess-A", "For A") //nolint:errcheck
	s.Create(ctx, "r-2", "local:sess-B", "For B") //nolint:errcheck

	repliesA, _ := s.ListBySession(ctx, "local:sess-A")
	if len(repliesA) != 1 || repliesA[0].Content != "For A" {
		t.Errorf("session A isolation failed: %+v", repliesA)
	}
}

func TestLocalReplyStore_DeleteBySession(t *testing.T) {
	s := setupLocalReplyDB(t)
	ctx := context.Background()

	s.Create(ctx, "r-1", "local:sess-1", "Reply 1") //nolint:errcheck
	s.Create(ctx, "r-2", "local:sess-1", "Reply 2") //nolint:errcheck

	if err := s.DeleteBySession(ctx, "local:sess-1"); err != nil {
		t.Fatalf("DeleteBySession: %v", err)
	}

	replies, _ := s.ListBySession(ctx, "local:sess-1")
	if len(replies) != 0 {
		t.Errorf("expected 0 replies after delete, got %d", len(replies))
	}
}

func TestLocalReplyStore_ListBySession_EmptyReturnsSlice(t *testing.T) {
	s := setupLocalReplyDB(t)
	replies, err := s.ListBySession(context.Background(), "local:nobody")
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if replies == nil {
		t.Error("expected non-nil slice, got nil")
	}
}
