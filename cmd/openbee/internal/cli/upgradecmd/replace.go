package upgradecmd

import (
	"fmt"
	"os"
	"runtime"
)

// oldBinarySuffix marks the previous binary moved aside during a Windows upgrade.
const oldBinarySuffix = ".old"

// replaceExecutable installs newPath at execPath. On Unix a rename atomically
// replaces the file even while it is running. Windows refuses to overwrite a
// running .exe but does allow renaming it, so the old binary is moved aside first.
func replaceExecutable(newPath, execPath string) error {
	if runtime.GOOS == "windows" {
		return replaceByMovingAside(newPath, execPath)
	}
	return os.Rename(newPath, execPath)
}

// replaceByMovingAside renames execPath to execPath+".old", then moves newPath
// into place, restoring the original if that second step fails. The ".old" file
// stays locked while the old process runs; the next upgrade removes it.
func replaceByMovingAside(newPath, execPath string) error {
	oldPath := execPath + oldBinarySuffix
	if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove leftover %s (is an old openbee process still running?): %w", oldPath, err)
	}
	if err := os.Rename(execPath, oldPath); err != nil {
		return fmt.Errorf("move current binary aside: %w", err)
	}
	if err := os.Rename(newPath, execPath); err != nil {
		if rbErr := os.Rename(oldPath, execPath); rbErr != nil {
			return fmt.Errorf("install new binary: %w (restore failed: %v; previous binary is at %s)", err, rbErr, oldPath)
		}
		return fmt.Errorf("install new binary: %w", err)
	}
	return nil
}
