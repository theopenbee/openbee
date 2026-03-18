package store_test

import (
	"context"
	"testing"

	"github.com/theopenbee/openbee/internal/store"
)

func TestMessageStore_ListBySessionKey_ExcludesMerged(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()
	s := store.NewMessageStore(db)
	ctx := context.Background()

	// Insert one received and one merged message for the same session
	s.CreateBatch(ctx, []store.BatchMsg{ //nolint:errcheck
		{ID: "m1", SessionKey: "local:s1", Platform: "local", Content: "hello",
			Status: "received", MessageTime: 1000},
		{ID: "m2", SessionKey: "local:s1", Platform: "local", Content: "world",
			Status: "merged", MergedInto: "m1", MessageTime: 900},
	})

	msgs, err := s.ListBySessionKey(ctx, "local:s1")
	if err != nil {
		t.Fatalf("ListBySessionKey: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 non-merged message, got %d", len(msgs))
	}
	if msgs[0].ID != "m1" {
		t.Errorf("expected message m1, got %s", msgs[0].ID)
	}
}

func TestMessageStore_DeleteBySessionKey(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()
	s := store.NewMessageStore(db)
	ctx := context.Background()

	s.CreateBatch(ctx, []store.BatchMsg{ //nolint:errcheck
		{ID: "m1", SessionKey: "local:s1", Platform: "local", Content: "a",
			Status: "received", MessageTime: 1000},
		{ID: "m2", SessionKey: "local:s2", Platform: "local", Content: "b",
			Status: "received", MessageTime: 1000},
	})

	if err := s.DeleteBySessionKey(ctx, "local:s1"); err != nil {
		t.Fatalf("DeleteBySessionKey: %v", err)
	}

	msgs, _ := s.ListBySessionKey(ctx, "local:s1")
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for s1, got %d", len(msgs))
	}
	msgs2, _ := s.ListBySessionKey(ctx, "local:s2")
	if len(msgs2) != 1 {
		t.Errorf("s2 should be unaffected, got %d", len(msgs2))
	}
}
