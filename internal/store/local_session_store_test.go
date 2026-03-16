package store_test

import (
	"context"
	"testing"

	"github.com/robobee/core/internal/store"
)

func setupLocalSessionDB(t *testing.T) *store.LocalSessionStore {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store.NewLocalSessionStore(db)
}

func TestLocalSessionStore_CreateAndList(t *testing.T) {
	s := setupLocalSessionDB(t)
	ctx := context.Background()

	if err := s.Create(ctx, "id-1", "My Session"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	sessions, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ID != "id-1" || sessions[0].Name != "My Session" {
		t.Errorf("unexpected session: %+v", sessions[0])
	}
}

func TestLocalSessionStore_List_OrdersByUpdatedAtDesc(t *testing.T) {
	s := setupLocalSessionDB(t)
	ctx := context.Background()

	s.Create(ctx, "id-1", "First")  //nolint:errcheck
	s.Create(ctx, "id-2", "Second") //nolint:errcheck
	s.TouchUpdatedAt(ctx, "id-1")   //nolint:errcheck

	sessions, _ := s.List(ctx)
	if sessions[0].ID != "id-1" {
		t.Errorf("expected id-1 first (most recently updated), got %s", sessions[0].ID)
	}
}

func TestLocalSessionStore_Delete(t *testing.T) {
	s := setupLocalSessionDB(t)
	ctx := context.Background()

	s.Create(ctx, "id-1", "To Delete") //nolint:errcheck
	if err := s.Delete(ctx, "id-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	sessions, _ := s.List(ctx)
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions after delete, got %d", len(sessions))
	}
}

func TestLocalSessionStore_List_EmptyReturnsSlice(t *testing.T) {
	s := setupLocalSessionDB(t)
	sessions, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if sessions == nil {
		t.Error("expected non-nil slice, got nil")
	}
}
