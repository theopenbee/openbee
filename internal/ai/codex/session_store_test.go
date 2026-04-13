package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionStore_GetMissing(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{dir: dir}
	_, ok := store.Get("nonexistent-uuid")
	if ok {
		t.Fatal("expected ok=false for missing session")
	}
}

func TestSessionStore_SetAndGet(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{dir: dir}

	if err := store.Set("openbee-uuid-1", "codex-thread-abc"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	threadID, ok := store.Get("openbee-uuid-1")
	if !ok {
		t.Fatal("expected ok=true after Set")
	}
	if threadID != "codex-thread-abc" {
		t.Errorf("got %q, want %q", threadID, "codex-thread-abc")
	}
}

func TestSessionStore_SetOverwrite(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{dir: dir}

	store.Set("uuid-1", "thread-v1")
	store.Set("uuid-1", "thread-v2")

	threadID, ok := store.Get("uuid-1")
	if !ok || threadID != "thread-v2" {
		t.Errorf("got (%q, %v), want (thread-v2, true)", threadID, ok)
	}
}

func TestSessionStore_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{dir: dir}

	if err := store.Set("uuid-atomic", "thread-xyz"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// No temp files should be left behind
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestNewSessionStore(t *testing.T) {
	// Use a non-existent subdir to verify auto-creation
	parent := t.TempDir()
	dir := filepath.Join(parent, "deep", "sessions")
	store, err := newSessionStoreAt(dir)
	if err != nil {
		t.Fatalf("newSessionStoreAt: %v", err)
	}
	if _, statErr := os.Stat(store.dir); statErr != nil {
		t.Errorf("sessions dir not created: %v", statErr)
	}
}
