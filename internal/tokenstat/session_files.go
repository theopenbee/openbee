package tokenstat

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

var ErrSessionDataNotFound = errors.New("tokenstat session data not found")

var errStopWalk = errors.New("stop walk")

// 16 MiB: session files can embed base64-encoded content or long conversation turns.
const scannerBufSize = 16 * 1024 * 1024

func getOrCreate(agg map[string]*SessionTokenUsage, sessionID, agentType, model string) *SessionTokenUsage {
	if u, ok := agg[model]; ok {
		return u
	}
	u := &SessionTokenUsage{SessionID: sessionID, AgentType: agentType, Model: model}
	agg[model] = u
	return u
}

func mapValues(agg map[string]*SessionTokenUsage) []SessionTokenUsage {
	result := make([]SessionTokenUsage, 0, len(agg))
	for _, u := range agg {
		result = append(result, *u)
	}
	return result
}

func scanJSONLFile(path string, fn func([]byte)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, scannerBufSize), scannerBufSize)
	for scanner.Scan() {
		fn(scanner.Bytes())
	}
	return scanner.Err()
}

func findWithLegacyFast(dir, legacyName string, match func(string, fs.DirEntry) bool) (string, error) {
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
			return "", fmt.Errorf("%w: %s", ErrSessionDataNotFound, root)
		}
		return "", fmt.Errorf("walk session root %s: %w", root, err)
	}
	if found == "" {
		return "", fmt.Errorf("%w: %s", ErrSessionDataNotFound, root)
	}
	return found, nil
}
