package linear

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSeenComments_LoadMissingReturnsEmpty(t *testing.T) {
	s := NewSeenComments(t.TempDir())
	if err := s.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Contains("anything") {
		t.Error("expected empty set after missing file load")
	}
}

func TestSeenComments_LoadCorruptFileTreatedAsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "seen_comments.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewSeenComments(dir)
	if err := s.Load(context.Background()); err != nil {
		t.Fatalf("Load corrupt: %v", err)
	}
	if s.Contains("anything") {
		t.Error("expected empty set after corrupt file")
	}
}

func TestSeenComments_ContainsReturnsFalseForUnknownID(t *testing.T) {
	s := NewSeenComments(t.TempDir())
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Contains("unknown-id") {
		t.Error("Contains returned true for unknown ID")
	}
}

func TestSeenComments_AddAndContainsRoundtrip(t *testing.T) {
	s := NewSeenComments(t.TempDir())
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(context.Background(), []string{"id-1", "id-2"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !s.Contains("id-1") || !s.Contains("id-2") {
		t.Error("Contains returned false after Add")
	}
	if s.Contains("id-3") {
		t.Error("Contains returned true for unadded ID")
	}
}

func TestSeenComments_AddPersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	s1 := NewSeenComments(dir)
	if err := s1.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s1.Add(context.Background(), []string{"id-1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Fresh instance — must restore id-1 from disk.
	s2 := NewSeenComments(dir)
	if err := s2.Load(context.Background()); err != nil {
		t.Fatalf("Load reload: %v", err)
	}
	if !s2.Contains("id-1") {
		t.Error("id-1 not found after reload")
	}
}

func TestSeenComments_AddLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	s := NewSeenComments(dir)
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(context.Background(), []string{"id-1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "seen_comments.json.tmp")); !os.IsNotExist(err) {
		t.Error("seen_comments.json.tmp should be removed after rename")
	}
}

func TestSeenComments_AddCreatesDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "nested", "linear")
	s := NewSeenComments(dir)
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(context.Background(), []string{"id-1"}); err != nil {
		t.Fatalf("Add into missing dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "seen_comments.json")); err != nil {
		t.Errorf("seen_comments.json not written: %v", err)
	}
}
