package utils

import (
	"io/fs"
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

func TestMoveFile_RenamesSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))
	dst := filepath.Join(dir, "dst.txt")

	require.NoError(t, MoveFile(src, dst))

	assert.NoFileExists(t, src)
	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "data", string(data))
}

func TestMoveFile_MissingSourceFails(t *testing.T) {
	dir := t.TempDir()

	err := MoveFile(filepath.Join(dir, "missing"), filepath.Join(dir, "dst.txt"))

	require.ErrorIs(t, err, fs.ErrNotExist)
	assert.NoFileExists(t, filepath.Join(dir, "dst.txt"))
}
