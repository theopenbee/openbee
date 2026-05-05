package linear

import (
	"bufio"
	"bytes"
	"context"
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
// <dir>/<filename> as NDJSON (one ID per line). Add is append-only.
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

// Load reads the persisted ID set from disk. A missing file silently
// yields an empty set. A trailing line without a newline is treated as
// crash debris and dropped.
func (s *SeenSet) Load(_ context.Context) error {
	data, err := os.ReadFile(filepath.Join(s.dir, s.filename))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		s.ids[line] = struct{}{}
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		if i := bytes.LastIndexByte(data, '\n'); i >= 0 {
			delete(s.ids, string(data[i+1:]))
		} else {
			delete(s.ids, string(data))
		}
	}
	return nil
}

// Contains reports whether id has already been dispatched.
func (s *SeenSet) Contains(id string) bool {
	_, ok := s.ids[id]
	return ok
}

// Add records ids as dispatched and appends only the previously-unseen
// IDs to the on-disk NDJSON file. Empty input and fully-duplicate input
// are no-ops.
func (s *SeenSet) Add(_ context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	fresh := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := s.ids[id]; ok {
			continue
		}
		s.ids[id] = struct{}{}
		fresh = append(fresh, id)
	}
	if len(fresh) == 0 {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	var buf bytes.Buffer
	for _, id := range fresh {
		buf.WriteString(id)
		buf.WriteByte('\n')
	}
	f, err := os.OpenFile(
		filepath.Join(s.dir, s.filename),
		os.O_WRONLY|os.O_CREATE|os.O_APPEND,
		0o600,
	)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(buf.Bytes())
	return err
}
