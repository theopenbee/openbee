package sessionfile_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/theopenbee/openbee/internal/utils/sessionfile"
)

func TestScanJSONLFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	content := "line one\nline two\nline three\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var lines []string
	if err := sessionfile.ScanJSONLFile(path, func(b []byte) {
		lines = append(lines, string(b))
	}); err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := []string{"line one", "line two", "line three"}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d", len(lines), len(want))
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestFindWithLegacyFast_LegacyHit(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "abc.jsonl")
	if err := os.WriteFile(legacy, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := sessionfile.FindWithLegacyFast(dir, "abc.jsonl", func(_ string, d os.DirEntry) bool {
		return d.Name() == "abc.jsonl"
	})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got != legacy {
		t.Errorf("got %q, want %q", got, legacy)
	}
}

func TestFindWithLegacyFast_NestedHit(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "sub", "sess-42.jsonl")
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(nested, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := sessionfile.FindWithLegacyFast(dir, "sess-42.jsonl", func(_ string, d os.DirEntry) bool {
		return d.Name() == "sess-42.jsonl"
	})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got != nested {
		t.Errorf("got %q, want %q", got, nested)
	}
}

func TestFindWithLegacyFast_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := sessionfile.FindWithLegacyFast(dir, "missing.jsonl", func(_ string, _ os.DirEntry) bool { return false })
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("got err %v, want wraps fs.ErrNotExist", err)
	}
}
