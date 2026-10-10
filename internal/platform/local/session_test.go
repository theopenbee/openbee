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

type claimCall struct{ from, to string }

type fakeClaimStore struct {
	owners map[string]string
	claims []claimCall
	err    error
}

func (f *fakeClaimStore) SessionKeyClaim(_ context.Context, from string) (string, bool, error) {
	owner, ok := f.owners[from]
	return owner, ok, f.err
}

func (f *fakeClaimStore) ClaimSessionKey(_ context.Context, from, to string) (string, error) {
	f.claims = append(f.claims, claimCall{from, to})
	if f.err != nil {
		return "", f.err
	}
	if owner, ok := f.owners[from]; ok {
		return owner, nil
	}
	if f.owners == nil {
		f.owners = map[string]string{}
	}
	f.owners[from] = to
	return to, nil
}

func noOwner() (string, bool, error) { return "", false, nil }

func ownerFinder(userID string) func() (string, bool, error) {
	return func() (string, bool, error) { return userID, true, nil }
}

func writeLegacyMedia(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, "default")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
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

func TestLegacySession_ClaimMovesMediaAndResolvesLegacyKey(t *testing.T) {
	root := t.TempDir()
	writeLegacyMedia(t, root, "old.png", "old")
	claims := &fakeClaimStore{}
	legacy := local.NewLegacySession(claims, root)

	assert.Equal(t, "local:default", legacy.Resolve("local:default"))
	require.NoError(t, legacy.Claim(context.Background(), "user-1"))

	data, err := os.ReadFile(filepath.Join(root, "user-1", "old.png"))
	require.NoError(t, err)
	assert.Equal(t, "old", string(data))
	assert.NoDirExists(t, filepath.Join(root, "default"))
	assert.Equal(t, []claimCall{{"local:default", "local:user-1"}}, claims.claims)
	assert.Equal(t, "local:user-1", legacy.Resolve("local:default"))
	assert.Equal(t, "local:user-2", legacy.Resolve("local:user-2"))
}

func TestLegacySession_ClaimKeepsRecordedOwner(t *testing.T) {
	root := t.TempDir()
	claims := &fakeClaimStore{owners: map[string]string{"local:default": "local:user-1"}}
	legacy := local.NewLegacySession(claims, root)
	writeLegacyMedia(t, root, "late.png", "late")

	require.NoError(t, legacy.Claim(context.Background(), "user-2"))

	assert.Equal(t, "local:user-1", legacy.Resolve("local:default"))
	assert.FileExists(t, filepath.Join(root, "user-1", "late.png"))
	assert.NoDirExists(t, filepath.Join(root, "user-2"))
}

func TestLegacySession_ClaimReturnsStoreError(t *testing.T) {
	legacy := local.NewLegacySession(&fakeClaimStore{err: errors.New("db down")}, t.TempDir())

	err := legacy.Claim(context.Background(), "user-1")

	require.ErrorContains(t, err, "db down")
	assert.Equal(t, "local:default", legacy.Resolve("local:default"))
}

func TestLegacySession_ClaimRecordsOwnerEvenWhenMediaMoveFails(t *testing.T) {
	root := t.TempDir()
	writeLegacyMedia(t, root, "a.png", "a")
	writeLegacyMedia(t, root, "b.png", "b")
	blocked := filepath.Join(root, "user-1", "a.png")
	require.NoError(t, os.MkdirAll(filepath.Join(blocked, "child"), 0o755))
	claims := &fakeClaimStore{}
	legacy := local.NewLegacySession(claims, root)

	err := legacy.Claim(context.Background(), "user-1")

	require.ErrorContains(t, err, "move legacy media")
	assert.Equal(t, []claimCall{{"local:default", "local:user-1"}}, claims.claims)
	assert.Equal(t, "local:user-1", legacy.Resolve("local:default"))
	assert.FileExists(t, filepath.Join(root, "user-1", "b.png"))
	assert.FileExists(t, filepath.Join(root, "default", "a.png"))
}

func TestLegacySession_RestoreClaimsForFirstOwner(t *testing.T) {
	claims := &fakeClaimStore{}
	legacy := local.NewLegacySession(claims, t.TempDir())

	require.NoError(t, legacy.Restore(context.Background(), ownerFinder("user-1")))

	assert.Equal(t, []claimCall{{"local:default", "local:user-1"}}, claims.claims)
	assert.Equal(t, "local:user-1", legacy.Resolve("local:default"))
}

func TestLegacySession_RestoreWithoutOwnerDoesNothing(t *testing.T) {
	claims := &fakeClaimStore{}
	legacy := local.NewLegacySession(claims, t.TempDir())

	require.NoError(t, legacy.Restore(context.Background(), noOwner))

	assert.Empty(t, claims.claims)
	assert.Equal(t, "local:default", legacy.Resolve("local:default"))
}

func TestLegacySession_RestoreUsesRecordedClaimWithoutReclaiming(t *testing.T) {
	root := t.TempDir()
	writeLegacyMedia(t, root, "left.png", "left")
	claims := &fakeClaimStore{owners: map[string]string{"local:default": "local:user-1"}}
	legacy := local.NewLegacySession(claims, root)

	require.NoError(t, legacy.Restore(context.Background(), ownerFinder("user-2")))

	assert.Empty(t, claims.claims)
	assert.Equal(t, "local:user-1", legacy.Resolve("local:default"))
	assert.FileExists(t, filepath.Join(root, "user-1", "left.png"))
}

func TestLegacySession_NilResolvesUnchanged(t *testing.T) {
	var legacy *local.LegacySession

	assert.Equal(t, "local:default", legacy.Resolve("local:default"))
}
