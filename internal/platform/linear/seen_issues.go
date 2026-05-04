package linear

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// seenIssuesAPI is satisfied by *SeenIssues and by test fakes.
type seenIssuesAPI interface {
	Load(ctx context.Context) error
	Contains(id string) bool
	Add(ctx context.Context, ids []string) error
}

// SeenIssues persists the set of already-dispatched issue IDs to
// <dir>/seen_issues.json. Writes use tmp+rename for atomicity.
// The set only grows; issues that leave the configured states are not removed.
type SeenIssues struct {
	dir string
	ids map[string]struct{}
}

// NewSeenIssues constructs a SeenIssues that persists to <dir>/seen_issues.json.
// Call Load before using Contains or Add.
func NewSeenIssues(dir string) *SeenIssues {
	return &SeenIssues{dir: dir, ids: make(map[string]struct{})}
}

type seenIssuesFile struct {
	IDs []string `json:"ids"`
}

// Load reads the persisted ID set. ErrNotExist or corrupt JSON yields an
// empty set silently — same fallback as SeenComments.
func (s *SeenIssues) Load(_ context.Context) error {
	data, err := os.ReadFile(filepath.Join(s.dir, "seen_issues.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var sf seenIssuesFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil // corrupt → empty
	}
	for _, id := range sf.IDs {
		s.ids[id] = struct{}{}
	}
	return nil
}

// Contains reports whether id has already been dispatched.
func (s *SeenIssues) Contains(id string) bool {
	_, ok := s.ids[id]
	return ok
}

// Add records ids as dispatched and atomically persists the full set.
// An empty input is a no-op (no disk write).
func (s *SeenIssues) Add(_ context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		s.ids[id] = struct{}{}
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	all := make([]string, 0, len(s.ids))
	for id := range s.ids {
		all = append(all, id)
	}
	data, err := json.Marshal(seenIssuesFile{IDs: all})
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, "seen_issues.json.tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, "seen_issues.json"))
}
