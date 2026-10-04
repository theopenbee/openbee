package codex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionStore_GetMissing(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{dir: dir}
	_, ok := store.Get("nonexistent-uuid")
	require.False(t, ok, "expected ok=false for missing session")
}

func TestSessionStore_SetAndGet(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{dir: dir}

	require.NoError(t, store.Set("openbee-uuid-1", "codex-thread-abc"))

	threadID, ok := store.Get("openbee-uuid-1")
	require.True(t, ok, "expected ok=true after Set")
	assert.Equal(t, "codex-thread-abc", threadID)
}

func TestSessionStore_SetOverwrite(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{dir: dir}

	store.Set("uuid-1", "thread-v1")
	store.Set("uuid-1", "thread-v2")

	threadID, ok := store.Get("uuid-1")
	assert.True(t, ok)
	assert.Equal(t, "thread-v2", threadID)
}

func TestSessionStore_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{dir: dir}

	require.NoError(t, store.Set("uuid-atomic", "thread-xyz"))

	// No temp files should be left behind
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		assert.NotEqual(t, ".tmp", filepath.Ext(e.Name()), "temp file left behind: %s", e.Name())
	}
}

func TestNewSessionStore(t *testing.T) {
	// Use a non-existent subdir to verify auto-creation
	parent := t.TempDir()
	dir := filepath.Join(parent, "deep", "sessions")
	store, err := newSessionStoreAt(dir)
	require.NoError(t, err)
	_, statErr := os.Stat(store.dir)
	assert.NoError(t, statErr, "sessions dir not created")
}
