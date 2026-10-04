package upgradecmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644), "write %s", path)
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err, "read %s", path)
	require.Equal(t, want, string(got), "%s", path)
}

// TestReplaceByMovingAside merges the basic replace and the two leftover-file
// variants: each pre-seeds zero or more stale "<execPath><suffix>" files
// before replacing, then expects the new binary to land at execPath and the
// previous binary to land at execPath+".old".
func TestReplaceByMovingAside(t *testing.T) {
	cases := []struct {
		name string
		// leftovers are pre-existing "<execPath><suffix>" files written before
		// replacing; each must either be overwritten (".old") or removed.
		leftovers []string
		// unrelatedFile, if set, is a differently-named file (not an
		// execPath-prefixed leftover) that must survive untouched.
		unrelatedFile string
	}{
		{
			name: "Basic",
		},
		{
			name:      "RemovesLeftover",
			leftovers: []string{".old"},
		},
		{
			name:          "RemovesUniqueLeftovers",
			leftovers:     []string{".old-1700000000"},
			unrelatedFile: "openbee.exe.older",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			execPath := filepath.Join(dir, "openbee.exe")
			newPath := filepath.Join(dir, ".openbee-new-123")
			writeTestFile(t, execPath, "old")
			writeTestFile(t, newPath, "new")
			for _, suffix := range tc.leftovers {
				writeTestFile(t, execPath+suffix, "stale-from-previous-upgrade")
			}
			var unrelated string
			if tc.unrelatedFile != "" {
				unrelated = filepath.Join(dir, tc.unrelatedFile)
				writeTestFile(t, unrelated, "keep")
			}

			err := replaceByMovingAside(newPath, execPath)
			require.NoError(t, err)
			assertFileContent(t, execPath, "new")
			assertFileContent(t, execPath+".old", "old")
			_, err = os.Stat(newPath)
			require.True(t, os.IsNotExist(err), "new binary should have been moved, stat err = %v", err)
			for _, suffix := range tc.leftovers {
				if suffix == ".old" {
					continue // already verified above: its stale content was replaced
				}
				_, err := os.Stat(execPath + suffix)
				require.True(t, os.IsNotExist(err), "leftover %s should have been removed, stat err = %v", suffix, err)
			}
			if unrelated != "" {
				assertFileContent(t, unrelated, "keep")
			}
		})
	}
}

func TestReplaceByMovingAsideRestoresOnFailure(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "openbee.exe")
	writeTestFile(t, execPath, "old")
	missingNew := filepath.Join(dir, "does-not-exist")

	err := replaceByMovingAside(missingNew, execPath)
	require.Error(t, err, "replaceByMovingAside with missing new binary: want a restored failure")
	require.NotErrorIs(t, err, errBinaryStranded, "replaceByMovingAside with missing new binary: want a restored failure")
	assertFileContent(t, execPath, "old")
	_, err = os.Stat(execPath + ".old")
	require.True(t, os.IsNotExist(err), ".old should not remain after restore, stat err = %v", err)
}

func TestReplaceByMovingAsideLeftoverStillInUse(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "openbee.exe")
	newPath := filepath.Join(dir, ".openbee-new-123")
	writeTestFile(t, execPath, "old")
	writeTestFile(t, newPath, "new")
	// A non-empty directory can't be removed on any OS, standing in for a
	// Windows .old file still locked by a running process.
	locked := filepath.Join(execPath+".old", "locked")
	require.NoError(t, os.MkdirAll(locked, 0o755))

	err := replaceByMovingAside(newPath, execPath)
	require.NoError(t, err, "replaceByMovingAside with undeletable .old")
	assertFileContent(t, execPath, "new")
	_, err = os.Stat(locked)
	require.NoError(t, err, "locked .old should be left alone")
	aside, err := filepath.Glob(execPath + ".old-*")
	require.NoError(t, err)
	require.Len(t, aside, 1, "want exactly one %s.old-* file, got %v", execPath, aside)
	assertFileContent(t, aside[0], "old")
}

func TestReplaceByMovingAsideKeepsBothBinariesWhenRestoreFails(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "openbee.exe")
	newPath := filepath.Join(dir, ".openbee-new-123")
	writeTestFile(t, execPath, "old")
	writeTestFile(t, newPath, "new")
	// Simulate execPath being locked right after the current binary moved away,
	// so neither the new binary nor the restore can land there.
	orig := renameFile
	renameFile = func(from, to string) error {
		if to == execPath {
			return &os.LinkError{Op: "rename", Old: from, New: to, Err: os.ErrPermission}
		}
		return orig(from, to)
	}
	t.Cleanup(func() { renameFile = orig })

	err := replaceByMovingAside(newPath, execPath)
	require.ErrorIs(t, err, errBinaryStranded, "replaceByMovingAside with failed restore")
	for _, p := range []string{newPath, execPath + ".old"} {
		require.Contains(t, err.Error(), p)
	}
	assertFileContent(t, newPath, "new")
	assertFileContent(t, execPath+".old", "old")
}
