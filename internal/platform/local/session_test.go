package local_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/platform/local"
)

type reassignCall struct{ from, to string }

type fakeReassigner struct {
	calls []reassignCall
	err   error
}

func (f *fakeReassigner) ReassignSessionKey(_ context.Context, from, to string) error {
	f.calls = append(f.calls, reassignCall{from, to})
	return f.err
}

func TestMediaDir(t *testing.T) {
	root := t.TempDir()

	dir, err := local.MediaDir(root, local.SessionKey("user-1"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "user-1"), dir)

	for _, key := range []string{"feishu:user-1", "local:", "local:..", "local:a/b", `local:a\b`} {
		_, err := local.MediaDir(root, key)
		assert.Error(t, err, key)
	}
}

func TestClaimLegacySession_MovesMediaAndReassignsSession(t *testing.T) {
	root := t.TempDir()
	legacyDir := filepath.Join(root, "default")
	require.NoError(t, os.MkdirAll(legacyDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacyDir, "old.png"), []byte("old"), 0o644))
	sessions := &fakeReassigner{}

	require.NoError(t, local.ClaimLegacySession(context.Background(), sessions, root, "user-1"))

	data, err := os.ReadFile(filepath.Join(root, "user-1", "old.png"))
	require.NoError(t, err)
	assert.Equal(t, "old", string(data))
	assert.NoDirExists(t, legacyDir)
	assert.Equal(t, []reassignCall{{"local:default", "local:user-1"}}, sessions.calls)
}

func TestClaimLegacySession_WithoutLegacyMedia(t *testing.T) {
	root := t.TempDir()
	sessions := &fakeReassigner{}

	require.NoError(t, local.ClaimLegacySession(context.Background(), sessions, root, "user-1"))

	assert.NoDirExists(t, filepath.Join(root, "user-1"))
	assert.Equal(t, []reassignCall{{"local:default", "local:user-1"}}, sessions.calls)
}

func TestClaimLegacySession_ReturnsReassignError(t *testing.T) {
	sessions := &fakeReassigner{err: errors.New("db down")}

	err := local.ClaimLegacySession(context.Background(), sessions, t.TempDir(), "user-1")

	require.ErrorContains(t, err, "db down")
}
