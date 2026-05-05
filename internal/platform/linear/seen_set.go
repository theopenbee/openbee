package linear

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// seenAPI is satisfied by *SeenSet and by test fakes.
type seenAPI interface {
	Load(ctx context.Context) error
	Contains(id string) bool
	Add(ctx context.Context, ids []string) error
}

// SeenSet persists a grow-only set of already-dispatched IDs to
// <dir>/<filename>. Writes use tmp+rename for atomicity.
type SeenSet struct {
	dir      string
	filename string
	ids      map[string]struct{}
}

// NewSeenSet constructs a SeenSet that persists to <dir>/<filename>.
// Call Load before using Contains or Add.
func NewSeenSet(dir, filename string) *SeenSet {
	return &SeenSet{dir: dir, filename: filename, ids: make(map[string]struct{})}
}

type seenFile struct {
	IDs []string `json:"ids"`
}

// Load reads the persisted ID set from disk. ErrNotExist or corrupt JSON
// silently yields an empty set.
func (s *SeenSet) Load(_ context.Context) error {
	data, err := os.ReadFile(filepath.Join(s.dir, s.filename))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var sf seenFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil
	}
	for _, id := range sf.IDs {
		s.ids[id] = struct{}{}
	}
	return nil
}

// Contains reports whether id has already been dispatched.
func (s *SeenSet) Contains(id string) bool {
	_, ok := s.ids[id]
	return ok
}

// Add records ids as dispatched and atomically persists the full set.
// Empty input is a no-op.
func (s *SeenSet) Add(_ context.Context, ids []string) error {
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
	tmp := filepath.Join(s.dir, s.filename+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, s.filename))
}
