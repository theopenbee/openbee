package tokenstat

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

var ErrSessionDataNotFound = errors.New("tokenstat session data not found")

var errStopWalk = errors.New("stop walk")

// scannerBufSize is the max JSONL line size for session file parsers.
// Session files can embed base64-encoded content or long conversation turns.
const scannerBufSize = 16 * 1024 * 1024

func findSessionFile(root string, match func(path string, d fs.DirEntry) bool) (string, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", ErrSessionDataNotFound, root)
		}
		return "", fmt.Errorf("stat session root %s: %w", root, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s", ErrSessionDataNotFound, root)
	}

	var found string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if match(path, d) {
			found = path
			return errStopWalk
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopWalk) {
		return "", fmt.Errorf("walk session root %s: %w", root, err)
	}
	if found == "" {
		return "", fmt.Errorf("%w: %s", ErrSessionDataNotFound, root)
	}
	return found, nil
}
