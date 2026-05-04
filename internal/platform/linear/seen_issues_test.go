package linear

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSeenIssues_LoadMissingReturnsEmpty(t *testing.T) {
	s := NewSeenIssues(t.TempDir())
	if err := s.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Contains("anything") {
		t.Error("expected empty set after missing file load")
	}
}

func TestSeenIssues_LoadCorruptFileTreatedAsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "seen_issues.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewSeenIssues(dir)
	if err := s.Load(context.Background()); err != nil {
		t.Fatalf("Load corrupt: %v", err)
	}
	if s.Contains("anything") {
		t.Error("expected empty set after corrupt file")
	}
}

func TestSeenIssues_AddAndContainsRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s := NewSeenIssues(dir)
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(context.Background(), []string{"I1", "I2"}); err != nil {
		t.Fatal(err)
	}
	if !s.Contains("I1") || !s.Contains("I2") {
		t.Error("Contains returned false for added IDs")
	}

	s2 := NewSeenIssues(dir)
	if err := s2.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !s2.Contains("I1") || !s2.Contains("I2") {
		t.Error("post-reload Contains false")
	}
}

func TestSeenIssues_AddEmptySliceIsNoop(t *testing.T) {
	dir := t.TempDir()
	s := NewSeenIssues(dir)
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "seen_issues.json")); !os.IsNotExist(err) {
		t.Errorf("Add(nil) should not create file; stat err: %v", err)
	}
}

func TestSeenIssues_AddLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	s := NewSeenIssues(dir)
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(context.Background(), []string{"I1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "seen_issues.json.tmp")); !os.IsNotExist(err) {
		t.Error("seen_issues.json.tmp should be removed after rename")
	}
}

func TestSeenIssues_AddCreatesDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "nested", "linear")
	s := NewSeenIssues(dir)
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(context.Background(), []string{"I1"}); err != nil {
		t.Fatalf("Add into missing dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "seen_issues.json")); err != nil {
		t.Errorf("seen_issues.json not written: %v", err)
	}
}
