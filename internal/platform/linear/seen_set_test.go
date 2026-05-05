package linear

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSeenSet_LoadMissingReturnsEmpty(t *testing.T) {
	s := NewSeenSet(t.TempDir(), "seen.json")
	if err := s.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Contains("anything") {
		t.Error("expected empty set after missing file load")
	}
}

func TestSeenSet_LoadCorruptFileTreatedAsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "seen.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewSeenSet(dir, "seen.json")
	if err := s.Load(context.Background()); err != nil {
		t.Fatalf("Load corrupt: %v", err)
	}
	if s.Contains("anything") {
		t.Error("expected empty set after corrupt file")
	}
}

func TestSeenSet_AddAndContainsRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s := NewSeenSet(dir, "seen.json")
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

	s2 := NewSeenSet(dir, "seen.json")
	if err := s2.Load(context.Background()); err != nil {
		t.Fatalf("Load reload: %v", err)
	}
	if !s2.Contains("id-1") || !s2.Contains("id-2") {
		t.Error("post-reload Contains false")
	}
}

func TestSeenSet_AddEmptySliceIsNoop(t *testing.T) {
	dir := t.TempDir()
	s := NewSeenSet(dir, "seen.json")
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "seen.json")); !os.IsNotExist(err) {
		t.Errorf("Add(nil) should not create file; stat err: %v", err)
	}
}

func TestSeenSet_AddLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	s := NewSeenSet(dir, "seen.json")
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(context.Background(), []string{"id-1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "seen.json.tmp")); !os.IsNotExist(err) {
		t.Error("seen.json.tmp should be removed after rename")
	}
}

func TestSeenSet_AddCreatesDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "nested", "linear")
	s := NewSeenSet(dir, "seen.json")
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(context.Background(), []string{"id-1"}); err != nil {
		t.Fatalf("Add into missing dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "seen.json")); err != nil {
		t.Errorf("seen.json not written: %v", err)
	}
}
