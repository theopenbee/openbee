// Package sessionfile provides file discovery and JSONL scanning helpers.
package sessionfile

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// 16 MiB: session files can embed base64-encoded content or long conversation turns.
const scannerBufSize = 16 * 1024 * 1024

var errStopWalk = errors.New("stop walk")

// ScanJSONLFile streams `path` line by line, invoking `fn` with a copy of each
// line's bytes. Empty lines are still passed through.
func ScanJSONLFile(path string, fn func([]byte)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, scannerBufSize)
	for scanner.Scan() {
		fn(append([]byte(nil), scanner.Bytes()...))
	}
	return scanner.Err()
}

// FindWithLegacyFast first checks for a flat-layout file at dir/legacyName
// (the old session layout). If absent, it walks dir recursively and returns
// the first file for which match returns true.
//
// Returns fs.ErrNotExist (wrapped) when nothing matches or when the directory
// does not exist.
//
// The os.Stat probe before WalkDir is technically a TOCTOU pattern, but
// callers re-open by path immediately after, and the saved syscall on the
// legacy-hit happy path outweighs the negligible race window.
func FindWithLegacyFast(dir, legacyName string, match func(string, fs.DirEntry) bool) (string, error) {
	legacyPath := filepath.Join(dir, legacyName)
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath, nil
	}
	return findSessionFile(dir, match)
}

func findSessionFile(root string, match func(path string, d fs.DirEntry) bool) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
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
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", fs.ErrNotExist, root)
		}
		return "", fmt.Errorf("walk session root %s: %w", root, err)
	}
	if found == "" {
		return "", fmt.Errorf("%w: %s", fs.ErrNotExist, root)
	}
	return found, nil
}
