package linear

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Cursor reads/writes the Linear poller high-water mark from a JSON file
// at <dir>/cursor.json. The write path uses tmp+rename for atomicity so a
// crash mid-write cannot leave a corrupted file.
type Cursor struct {
	dir string
}

// NewCursor constructs a Cursor that persists to <dir>/cursor.json.
func NewCursor(dir string) *Cursor {
	return &Cursor{dir: dir}
}

type cursorFile struct {
	LastSync string `json:"last_sync"`
}

// Load returns the saved high-water mark, or now-1h on first run / corrupt file.
func (c *Cursor) Load(ctx context.Context) (time.Time, error) {
	data, err := os.ReadFile(filepath.Join(c.dir, "cursor.json"))
	if errors.Is(err, os.ErrNotExist) {
		return time.Now().Add(-1 * time.Hour), nil
	}
	if err != nil {
		return time.Time{}, err
	}
	var cf cursorFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return time.Now().Add(-1 * time.Hour), nil
	}
	t, err := time.Parse(time.RFC3339Nano, cf.LastSync)
	if err != nil {
		return time.Now().Add(-1 * time.Hour), nil
	}
	return t, nil
}

// Save persists the high-water mark atomically via tmp+rename.
func (c *Cursor) Save(ctx context.Context, t time.Time) error {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(cursorFile{LastSync: t.UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return err
	}
	tmp := filepath.Join(c.dir, "cursor.json.tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(c.dir, "cursor.json"))
}
