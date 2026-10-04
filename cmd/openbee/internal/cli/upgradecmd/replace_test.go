package upgradecmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func TestReplaceByMovingAside(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "openbee.exe")
	newPath := filepath.Join(dir, ".openbee-new-123")
	writeTestFile(t, execPath, "old")
	writeTestFile(t, newPath, "new")

	if err := replaceByMovingAside(newPath, execPath); err != nil {
		t.Fatalf("replaceByMovingAside: %v", err)
	}
	assertFileContent(t, execPath, "new")
	assertFileContent(t, execPath+".old", "old")
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatalf("new binary should have been moved, stat err = %v", err)
	}
}

func TestReplaceByMovingAsideRemovesLeftover(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "openbee.exe")
	newPath := filepath.Join(dir, ".openbee-new-123")
	writeTestFile(t, execPath, "old")
	writeTestFile(t, newPath, "new")
	writeTestFile(t, execPath+".old", "stale-from-previous-upgrade")

	if err := replaceByMovingAside(newPath, execPath); err != nil {
		t.Fatalf("replaceByMovingAside with leftover .old: %v", err)
	}
	assertFileContent(t, execPath, "new")
	assertFileContent(t, execPath+".old", "old")
}

func TestReplaceByMovingAsideRestoresOnFailure(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "openbee.exe")
	writeTestFile(t, execPath, "old")
	missingNew := filepath.Join(dir, "does-not-exist")

	if err := replaceByMovingAside(missingNew, execPath); err == nil {
		t.Fatalf("replaceByMovingAside with missing new binary returned nil error")
	}
	assertFileContent(t, execPath, "old")
	if _, err := os.Stat(execPath + ".old"); !os.IsNotExist(err) {
		t.Fatalf(".old should not remain after restore, stat err = %v", err)
	}
}

func TestReplaceByMovingAsideLeftoverStillInUse(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "openbee.exe")
	newPath := filepath.Join(dir, ".openbee-new-123")
	writeTestFile(t, execPath, "old")
	writeTestFile(t, newPath, "new")
	// A non-empty directory can't be removed on any OS, standing in for a
	// Windows .old file still locked by a running process.
	if err := os.MkdirAll(filepath.Join(execPath+".old", "locked"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	err := replaceByMovingAside(newPath, execPath)
	if err == nil || !strings.Contains(err.Error(), "still running") {
		t.Fatalf("replaceByMovingAside with undeletable .old: err = %v, want a still-running hint", err)
	}
	assertFileContent(t, execPath, "old")
	assertFileContent(t, newPath, "new")
}
