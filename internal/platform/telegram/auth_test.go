package telegram

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthStore_AuthorizeAndCheck(t *testing.T) {
	dir := t.TempDir()
	s := &AuthStore{
		users:    make(map[string]bool),
		filePath: filepath.Join(dir, "authorized_users.json"),
	}

	assert.False(t, s.IsAuthorized("123"), "user should not be authorized initially")

	s.Authorize("123")

	assert.True(t, s.IsAuthorized("123"), "user should be authorized after Authorize()")
	assert.False(t, s.IsAuthorized("456"), "other user should not be authorized")
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

	assert.True(t, s2.IsAuthorized("111"), "persisted users should be loadable")
	assert.True(t, s2.IsAuthorized("222"), "persisted users should be loadable")
	assert.False(t, s2.IsAuthorized("333"), "non-persisted user should not be authorized")
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
	require.NoError(t, err)

	var ids []string
	require.NoError(t, json.Unmarshal(data, &ids))
	assert.Equal(t, []string{"42"}, ids)
}

func TestAuthStore_LoadMissingFile(t *testing.T) {
	s := &AuthStore{
		users:    make(map[string]bool),
		filePath: filepath.Join(t.TempDir(), "nonexistent.json"),
	}
	s.load() // should not panic
	assert.False(t, s.IsAuthorized("any"), "empty store should authorize no one")
}
