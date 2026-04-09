package store_test

import (
	"context"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/store"
)

func TestMessageStore_ListBySessionKey_ExcludesMerged(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()
	s := store.NewMessageStore(db)
	ctx := context.Background()

	s.CreateBatch(ctx, []store.BatchMsg{ //nolint:errcheck
		{ID: "m1", SessionKey: "local:s1", Platform: "local", Content: "hello",
			Status: "received", MessageTime: 1000},
		{ID: "m2", SessionKey: "local:s1", Platform: "local", Content: "world",
			Status: "merged", MergedInto: "m1", MessageTime: 900},
	})

	msgs, err := s.ListBySessionKey(ctx, "local:s1", 0, 50)
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

func TestMessageStore_ListBySessionKey_Pagination(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()
	s := store.NewMessageStore(db)
	ctx := context.Background()

	// Insert 5 messages with timestamps 100..500
	s.CreateBatch(ctx, []store.BatchMsg{ //nolint:errcheck
		{ID: "a", SessionKey: "local:s1", Platform: "local", Content: "1", Status: "received", MessageTime: 100},
		{ID: "b", SessionKey: "local:s1", Platform: "local", Content: "2", Status: "received", MessageTime: 200},
		{ID: "c", SessionKey: "local:s1", Platform: "local", Content: "3", Status: "received", MessageTime: 300},
		{ID: "d", SessionKey: "local:s1", Platform: "local", Content: "4", Status: "received", MessageTime: 400},
		{ID: "e", SessionKey: "local:s1", Platform: "local", Content: "5", Status: "received", MessageTime: 500},
	})

	// limit=3, no before -> store fetches limit+1=4 rows (b,c,d,e ASC) so caller can detect has_more
	msgs, err := s.ListBySessionKey(ctx, "local:s1", 0, 3)
	if err != nil {
		t.Fatalf("ListBySessionKey: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages (limit+1 over-fetch), got %d", len(msgs))
	}
	if msgs[0].ID != "b" || msgs[3].ID != "e" {
		t.Errorf("expected b,c,d,e got %s,...,%s", msgs[0].ID, msgs[3].ID)
	}

	// before=300 (exclusive) -> store fetches limit+1=4 before ts 300, but only 2 exist (a,b)
	msgs2, err := s.ListBySessionKey(ctx, "local:s1", 300, 3)
	if err != nil {
		t.Fatalf("ListBySessionKey with before: %v", err)
	}
	if len(msgs2) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs2))
	}
	if msgs2[0].ID != "a" || msgs2[1].ID != "b" {
		t.Errorf("expected a,b got %s,%s", msgs2[0].ID, msgs2[1].ID)
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

	msgs, _ := s.ListBySessionKey(ctx, "local:s1", 0, 50)
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for s1, got %d", len(msgs))
	}
	msgs2, _ := s.ListBySessionKey(ctx, "local:s2", 0, 50)
	if len(msgs2) != 1 {
		t.Errorf("s2 should be unaffected, got %d", len(msgs2))
	}
}
