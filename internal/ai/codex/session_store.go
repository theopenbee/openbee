package codex

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/theopenbee/openbee/internal/infra/config"
)

// SessionStore maps openbee session UUIDs to codex thread IDs on disk.
// Per-session files avoid concurrent write conflicts between sessions.
type SessionStore struct {
	dir string
}

// NewSessionStore creates a SessionStore rooted at the default location.
func NewSessionStore() (*SessionStore, error) {
	return newSessionStoreAt(config.DefaultCodexSessionsDir())
}

func newSessionStoreAt(dir string) (*SessionStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}
	return &SessionStore{dir: dir}, nil
}

func (s *SessionStore) Get(openbeeUUID string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(s.dir, openbeeUUID))
	if err != nil {
		return "", false
	}
	threadID := string(data)
	if threadID == "" {
		return "", false
	}
	return threadID, true
}

// Set writes the codex thread_id for the given openbee UUID. Writes are atomic via temp-file rename.
func (s *SessionStore) Set(openbeeUUID, threadID string) error {
	dest := filepath.Join(s.dir, openbeeUUID)
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, []byte(threadID), 0o644); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("write temp session file: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename session file: %w", err)
	}
	return nil
}
