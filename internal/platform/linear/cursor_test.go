package linear

import (
	"context"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/infra/store"
)

func newCursorTestStore(t *testing.T) *store.SystemConfigStore {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store.NewSystemConfigStore(db)
}

func TestCursor_LoadMissingReturnsBootstrapWindow(t *testing.T) {
	c := NewCursor(newCursorTestStore(t))
	got, err := c.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	delta := time.Since(got)
	if delta < 30*time.Minute || delta > 90*time.Minute {
		t.Errorf("bootstrap window out of range: now-loaded=%v", delta)
	}
}

func TestCursor_SaveAndLoad(t *testing.T) {
	c := NewCursor(newCursorTestStore(t))
	want := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	if err := c.Save(context.Background(), want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := c.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
