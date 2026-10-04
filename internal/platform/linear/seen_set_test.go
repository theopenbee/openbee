package linear

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeenSet_LoadMissingReturnsEmpty(t *testing.T) {
	s := NewSeenSet(t.TempDir(), "seen.ndjson")
	require.NoError(t, s.Load(context.Background()))
	assert.False(t, s.Contains("anything"), "expected empty set after missing file load")
}

func TestSeenSet_AddAndContainsRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s := NewSeenSet(dir, "seen.ndjson")
	require.NoError(t, s.Load(context.Background()))
	require.NoError(t, s.Add(context.Background(), []string{"id-1", "id-2"}))
	assert.True(t, s.Contains("id-1"), "Contains returned false after Add")
	assert.True(t, s.Contains("id-2"), "Contains returned false after Add")
	assert.False(t, s.Contains("id-3"), "Contains returned true for unadded ID")

	s2 := NewSeenSet(dir, "seen.ndjson")
	require.NoError(t, s2.Load(context.Background()))
	assert.True(t, s2.Contains("id-1"), "post-reload Contains false")
	assert.True(t, s2.Contains("id-2"), "post-reload Contains false")
}

func TestSeenSet_AddEmptySliceIsNoop(t *testing.T) {
	dir := t.TempDir()
	s := NewSeenSet(dir, "seen.ndjson")
	require.NoError(t, s.Load(context.Background()))
	require.NoError(t, s.Add(context.Background(), nil))
	_, err := os.Stat(filepath.Join(dir, "seen.ndjson"))
	assert.True(t, os.IsNotExist(err), "Add(nil) should not create file; stat err: %v", err)
}

func TestSeenSet_AddCreatesDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "nested", "linear")
	s := NewSeenSet(dir, "seen.ndjson")
	require.NoError(t, s.Load(context.Background()))
	require.NoError(t, s.Add(context.Background(), []string{"id-1"}))
	_, err := os.Stat(filepath.Join(dir, "seen.ndjson"))
	assert.NoError(t, err, "seen.ndjson not written")
}

func TestSeenSet_AddWritesNDJSON(t *testing.T) {
	dir := t.TempDir()
	s := NewSeenSet(dir, "seen.ndjson")
	require.NoError(t, s.Load(context.Background()))
	require.NoError(t, s.Add(context.Background(), []string{"id-1", "id-2"}))
	data, err := os.ReadFile(filepath.Join(dir, "seen.ndjson"))
	require.NoError(t, err)
	assert.Equal(t, "id-1\nid-2\n", string(data))
}

// TestSeenSet_Add covers the write semantics of Add: it must append rather
// than rewrite the file, skip IDs already recorded across calls, and
// deduplicate IDs repeated within a single call.
func TestSeenSet_Add(t *testing.T) {
	cases := []struct {
		name string
		// batches are applied as successive Add calls.
		batches [][]string
		// rawRewrite, if non-empty, overwrites the on-disk file directly
		// (bypassing Add) right after the first batch — simulating
		// out-of-band content Add must not clobber on the next call.
		rawRewrite string
		want       []string
	}{
		{
			// Add must append, not rewrite: an out-of-band write to the file
			// between two Add calls must survive the next Add.
			name:       "IsAppendOnly",
			batches:    [][]string{{"id-1"}, {"id-2"}},
			rawRewrite: "SENTINEL\n",
			want:       []string{"SENTINEL", "id-2"},
		},
		{
			// Duplicates must not be re-appended across separate Add calls.
			name:    "SkipsAlreadySeen",
			batches: [][]string{{"id-1"}, {"id-1", "id-2"}},
			want:    []string{"id-1", "id-2"},
		},
		{
			// Duplicates within a single Add call must collapse before
			// writing.
			name:    "DeduplicatesWithinSingleCall",
			batches: [][]string{{"id-1", "id-1", "id-2"}},
			want:    []string{"id-1", "id-2"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			s := NewSeenSet(dir, "seen.ndjson")
			require.NoError(t, s.Load(context.Background()))

			path := filepath.Join(dir, "seen.ndjson")
			for i, batch := range tc.batches {
				require.NoError(t, s.Add(context.Background(), batch))
				if i == 0 && tc.rawRewrite != "" {
					require.NoError(t, os.WriteFile(path, []byte(tc.rawRewrite), 0o600))
				}
			}

			data, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, strings.Join(tc.want, "\n")+"\n", string(data))
		})
	}
}

func TestSeenSet_LoadIgnoresLonePartialLine(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "seen.ndjson"), []byte("only-id"), 0o600))
	s := NewSeenSet(dir, "seen.ndjson")
	require.NoError(t, s.Load(context.Background()))
	assert.False(t, s.Contains("only-id"), "a lone partial line (no newline anywhere) must be dropped on Load")
}

func TestSeenSet_AddDoesNotMutateMemoryOnWriteFailure(t *testing.T) {
	dir := t.TempDir()
	s := NewSeenSet(dir, "seen.ndjson")
	require.NoError(t, s.Load(context.Background()))
	// Replace the file path with a directory of the same name; OpenFile
	// for write will fail with EISDIR-equivalent on every platform.
	path := filepath.Join(dir, "seen.ndjson")
	require.NoError(t, os.Mkdir(path, 0o700))
	err := s.Add(context.Background(), []string{"id-1"})
	require.Error(t, err, "expected Add to fail when the target path is a directory")
	assert.False(t, s.Contains("id-1"), "on write failure, in-memory set must not contain the new ID")
}

func TestSeenSet_LoadIgnoresPartialTrailingLine(t *testing.T) {
	dir := t.TempDir()
	content := []byte("id-1\nid-2\nid-3-partial")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "seen.ndjson"), content, 0o600))
	s := NewSeenSet(dir, "seen.ndjson")
	require.NoError(t, s.Load(context.Background()))
	assert.True(t, s.Contains("id-1"), "complete IDs should be loaded")
	assert.True(t, s.Contains("id-2"), "complete IDs should be loaded")
	assert.False(t, s.Contains("id-3-partial"), "partial trailing line must be ignored on Load")
}
