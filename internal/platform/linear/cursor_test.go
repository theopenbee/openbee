package linear

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCursor_LoadMissingReturnsBootstrapWindow(t *testing.T) {
	c := NewCursor(t.TempDir())
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
	dir := t.TempDir()
	c := NewCursor(dir)
	want := time.Date(2026, 5, 2, 12, 0, 0, 123456789, time.UTC)
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

func TestCursor_LoadCorruptFileReturnsBootstrapWindow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cursor.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := NewCursor(dir).Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if delta := time.Since(got); delta < 30*time.Minute || delta > 90*time.Minute {
		t.Errorf("expected bootstrap fallback, got delta=%v", delta)
	}
}

func TestCursor_SaveLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	c := NewCursor(dir)
	if err := c.Save(context.Background(), time.Now()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cursor.json.tmp")); !os.IsNotExist(err) {
		t.Errorf("cursor.json.tmp should be removed after rename, stat err: %v", err)
	}
}

func TestCursor_SaveCreatesDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "nested", "linear")
	if err := NewCursor(dir).Save(context.Background(), time.Now()); err != nil {
		t.Fatalf("Save into missing dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cursor.json")); err != nil {
		t.Errorf("cursor.json not written: %v", err)
	}
}
