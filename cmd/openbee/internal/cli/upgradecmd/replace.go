package upgradecmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// oldBinarySuffix marks the previous binary moved aside during a Windows upgrade.
const oldBinarySuffix = ".old"

// errBinaryStranded marks a failed Windows swap that left no binary at the
// executable path. The caller must keep the new binary on disk so the user can
// move it (or the previous one) into place by hand.
var errBinaryStranded = errors.New("no binary left at the executable path")

// renameFile is os.Rename; tests replace it to simulate Windows file locks.
var renameFile = os.Rename

// replaceExecutable installs newPath at execPath. On Unix a rename atomically
// replaces the file even while it is running. Windows refuses to overwrite a
// running .exe but does allow renaming it, so the old binary is moved aside first.
func replaceExecutable(newPath, execPath string) error {
	if runtime.GOOS == "windows" {
		return replaceByMovingAside(newPath, execPath)
	}
	return os.Rename(newPath, execPath)
}

// replaceByMovingAside renames execPath out of the way (see asidePath), then moves
// newPath into place, restoring the original if that second step fails. The
// moved-aside binary stays locked while the old process runs; a later upgrade
// removes it.
func replaceByMovingAside(newPath, execPath string) error {
	oldPath := asidePath(execPath)
	if err := renameFile(execPath, oldPath); err != nil {
		return fmt.Errorf("move current binary aside: %w", err)
	}
	if err := renameFile(newPath, execPath); err != nil {
		if rbErr := renameFile(oldPath, execPath); rbErr != nil {
			return fmt.Errorf("%w: install new binary: %w; restore previous binary: %v; rename %s (new) or %s (previous) to %s by hand",
				errBinaryStranded, err, rbErr, newPath, oldPath, execPath)
		}
		return fmt.Errorf("install new binary: %w", err)
	}
	return nil
}

// asidePath removes binaries moved aside by earlier upgrades and returns where to
// move the current one: execPath+".old", or a fresh execPath+".old-<n>" while an
// old process still holds that file, so a running old daemon can't block upgrades.
func asidePath(execPath string) string {
	oldPath := execPath + oldBinarySuffix
	dir, oldName := filepath.Dir(oldPath), filepath.Base(oldPath)
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if n := e.Name(); n == oldName || strings.HasPrefix(n, oldName+"-") {
				_ = os.Remove(filepath.Join(dir, n)) // fails while still locked; retried next upgrade
			}
		}
	}
	if _, err := os.Lstat(oldPath); os.IsNotExist(err) {
		return oldPath
	}
	return fmt.Sprintf("%s-%d", oldPath, time.Now().UnixNano())
}
