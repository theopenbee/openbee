package sessionfile_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/utils/sessionfile"
)

func TestScanJSONLFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	content := "line one\nline two\nline three\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	var lines []string
	require.NoError(t, sessionfile.ScanJSONLFile(path, func(b []byte) {
		lines = append(lines, string(b))
	}))
	want := []string{"line one", "line two", "line three"}
	assert.Equal(t, want, lines)
}

func TestFindWithLegacyFast_LegacyHit(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "abc.jsonl")
	require.NoError(t, os.WriteFile(legacy, []byte("x"), 0o644))
	got, err := sessionfile.FindWithLegacyFast(dir, "abc.jsonl", func(_ string, d os.DirEntry) bool {
		return d.Name() == "abc.jsonl"
	})
	require.NoError(t, err)
	assert.Equal(t, legacy, got)
}

func TestFindWithLegacyFast_NestedHit(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "sub", "sess-42.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(nested), 0o755))
	require.NoError(t, os.WriteFile(nested, []byte("x"), 0o644))
	got, err := sessionfile.FindWithLegacyFast(dir, "sess-42.jsonl", func(_ string, d os.DirEntry) bool {
		return d.Name() == "sess-42.jsonl"
	})
	require.NoError(t, err)
	assert.Equal(t, nested, got)
}

func TestFindWithLegacyFast_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := sessionfile.FindWithLegacyFast(dir, "missing.jsonl", func(_ string, _ os.DirEntry) bool { return false })
	assert.ErrorIs(t, err, fs.ErrNotExist)
}
