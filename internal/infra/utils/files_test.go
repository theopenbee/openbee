package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyFile_CreatesParentDir(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.txt")
	require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))
	dst := filepath.Join(t.TempDir(), "nested", "dst.txt")

	require.NoError(t, CopyFile(src, dst))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "data", string(data))
}

func TestCopyFile_MissingSourceLeavesNoDestination(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "dst.txt")

	require.Error(t, CopyFile(filepath.Join(t.TempDir(), "missing"), dst))

	assert.NoFileExists(t, dst)
}

func TestLinkOrCopyFile_HardLinksSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))
	dst := filepath.Join(dir, "staged", "dst.txt")

	require.NoError(t, LinkOrCopyFile(src, dst))

	srcInfo, err := os.Stat(src)
	require.NoError(t, err)
	dstInfo, err := os.Stat(dst)
	require.NoError(t, err)
	assert.True(t, os.SameFile(srcInfo, dstInfo))
}

func TestLinkOrCopyFile_ReplacesExistingLinkWithoutTruncatingSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))
	dst := filepath.Join(dir, "dst.txt")
	require.NoError(t, os.Link(src, dst))

	require.NoError(t, LinkOrCopyFile(src, dst))

	data, err := os.ReadFile(src)
	require.NoError(t, err)
	assert.Equal(t, "data", string(data))
}

func TestLinkOrCopyFile_FollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("data"), 0o644))
	link := filepath.Join(dir, "link.txt")
	require.NoError(t, os.Symlink("target.txt", link))
	dst := filepath.Join(t.TempDir(), "dst.txt")

	require.NoError(t, LinkOrCopyFile(link, dst))

	info, err := os.Lstat(dst)
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&os.ModeSymlink)
	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "data", string(data))
}
