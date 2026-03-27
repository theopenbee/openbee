package backup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/backup"
)

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// write a file to hash
	f := filepath.Join(dir, "hello.txt")
	require.NoError(t, os.WriteFile(f, []byte("hello world"), 0644))

	sum, err := backup.SHA256File(f)
	require.NoError(t, err)
	require.Len(t, sum, 64) // hex-encoded sha256

	m := backup.Manifest{
		Version:        "1",
		OpenbeeVersion: "0.5.0",
		CreatedAt:      "2026-03-27T15:30:00Z",
		Files: []backup.FileEntry{
			{Path: "hello.txt", SHA256: sum},
		},
	}

	out := filepath.Join(dir, "manifest.json")
	require.NoError(t, backup.WriteManifest(out, m))

	got, err := backup.ReadManifest(out)
	require.NoError(t, err)
	require.Equal(t, m, got)
}
