package linear

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// seenAPI is satisfied by *SeenComments and by test fakes.
type seenAPI interface {
	Load(ctx context.Context) error
	Contains(id string) bool
	Add(ctx context.Context, ids []string) error
}

// SeenComments persists the set of already-dispatched comment IDs to
// <dir>/seen_comments.json. Writes use tmp+rename for atomicity.
type SeenComments struct {
	dir string
	ids map[string]struct{}
}

// NewSeenComments constructs a SeenComments that persists to <dir>/seen_comments.json.
// Call Load before using Contains or Add.
func NewSeenComments(dir string) *SeenComments {
	return &SeenComments{dir: dir, ids: make(map[string]struct{})}
}

type seenFile struct {
	IDs []string `json:"ids"`
}

// Load reads the persisted ID set from disk. On ErrNotExist or corrupt JSON
// it silently starts with an empty set (same fallback pattern as Cursor.Load).
func (s *SeenComments) Load(_ context.Context) error {
	data, err := os.ReadFile(filepath.Join(s.dir, "seen_comments.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var sf seenFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil // corrupt → empty
	}
	for _, id := range sf.IDs {
		s.ids[id] = struct{}{}
	}
	return nil
}

// Contains reports whether id has already been dispatched.
func (s *SeenComments) Contains(id string) bool {
	_, ok := s.ids[id]
	return ok
}

// Add records ids as dispatched and atomically persists the full set to disk.
func (s *SeenComments) Add(_ context.Context, ids []string) error {
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
	data, err := json.Marshal(seenFile{IDs: all})
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, "seen_comments.json.tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, "seen_comments.json"))
}
