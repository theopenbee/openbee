package upgradecmd

import (
	"errors"
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

	err := replaceByMovingAside(missingNew, execPath)
	if err == nil || errors.Is(err, errBinaryStranded) {
		t.Fatalf("replaceByMovingAside with missing new binary: err = %v, want a restored failure", err)
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
	locked := filepath.Join(execPath+".old", "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := replaceByMovingAside(newPath, execPath); err != nil {
		t.Fatalf("replaceByMovingAside with undeletable .old: %v", err)
	}
	assertFileContent(t, execPath, "new")
	if _, err := os.Stat(locked); err != nil {
		t.Fatalf("locked .old should be left alone, stat err = %v", err)
	}
	aside, err := filepath.Glob(execPath + ".old-*")
	if err != nil || len(aside) != 1 {
		t.Fatalf("want exactly one %s.old-* file, got %v (err %v)", execPath, aside, err)
	}
	assertFileContent(t, aside[0], "old")
}

func TestReplaceByMovingAsideRemovesUniqueLeftovers(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "openbee.exe")
	newPath := filepath.Join(dir, ".openbee-new-123")
	writeTestFile(t, execPath, "old")
	writeTestFile(t, newPath, "new")
	writeTestFile(t, execPath+".old-1700000000", "stale")
	unrelated := filepath.Join(dir, "openbee.exe.older")
	writeTestFile(t, unrelated, "keep")

	if err := replaceByMovingAside(newPath, execPath); err != nil {
		t.Fatalf("replaceByMovingAside: %v", err)
	}
	assertFileContent(t, execPath, "new")
	assertFileContent(t, execPath+".old", "old")
	if _, err := os.Stat(execPath + ".old-1700000000"); !os.IsNotExist(err) {
		t.Fatalf("leftover .old-* should have been removed, stat err = %v", err)
	}
	assertFileContent(t, unrelated, "keep")
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
	if !errors.Is(err, errBinaryStranded) {
		t.Fatalf("replaceByMovingAside with failed restore: err = %v, want errBinaryStranded", err)
	}
	for _, p := range []string{newPath, execPath + ".old"} {
		if !strings.Contains(err.Error(), p) {
			t.Fatalf("error %q does not name %s", err, p)
		}
	}
	assertFileContent(t, newPath, "new")
	assertFileContent(t, execPath+".old", "old")
}
