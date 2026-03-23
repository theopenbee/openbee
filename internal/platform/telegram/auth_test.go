package telegram

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthStore_AuthorizeAndCheck(t *testing.T) {
	dir := t.TempDir()
	s := &AuthStore{
		users:    make(map[string]bool),
		filePath: filepath.Join(dir, "authorized_users.json"),
	}

	if s.IsAuthorized("123") {
		t.Error("user should not be authorized initially")
	}

	s.Authorize("123")

	if !s.IsAuthorized("123") {
		t.Error("user should be authorized after Authorize()")
	}
	if s.IsAuthorized("456") {
		t.Error("other user should not be authorized")
	}
}

func TestAuthStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "authorized_users.json")

	// Write and persist.
	s1 := &AuthStore{
		users:    make(map[string]bool),
		filePath: fp,
	}
	s1.Authorize("111")
	s1.Authorize("222")

	// Load in a new store.
	s2 := &AuthStore{
		users:    make(map[string]bool),
		filePath: fp,
	}
	s2.load()

	if !s2.IsAuthorized("111") || !s2.IsAuthorized("222") {
		t.Error("persisted users should be loadable")
	}
	if s2.IsAuthorized("333") {
		t.Error("non-persisted user should not be authorized")
	}
}

func TestAuthStore_FileFormat(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "authorized_users.json")

	s := &AuthStore{
		users:    make(map[string]bool),
		filePath: fp,
	}
	s.Authorize("42")

	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(ids) != 1 || ids[0] != "42" {
		t.Errorf("file content = %v, want [\"42\"]", ids)
	}
}

func TestAuthStore_LoadMissingFile(t *testing.T) {
	s := &AuthStore{
		users:    make(map[string]bool),
		filePath: filepath.Join(t.TempDir(), "nonexistent.json"),
	}
	s.load() // should not panic
	if s.IsAuthorized("any") {
		t.Error("empty store should authorize no one")
	}
}
