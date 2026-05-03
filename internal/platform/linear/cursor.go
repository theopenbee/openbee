package linear

import (
	"context"
	"time"

	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

// Cursor reads/writes the Linear poller high-water mark from system_configs.
type Cursor struct {
	store *store.SystemConfigStore
}

// NewCursor constructs a Cursor backed by the given SystemConfigStore.
func NewCursor(s *store.SystemConfigStore) *Cursor {
	return &Cursor{store: s}
}

// Load returns the saved high-water mark, or now-1h on first run.
func (c *Cursor) Load(ctx context.Context) (time.Time, error) {
	cfg, found, err := c.store.Get(ctx, model.SystemConfigKeyLinearLastSync)
	if err != nil {
		return time.Time{}, err
	}
	if !found || cfg.Value == "" {
		return time.Now().Add(-1 * time.Hour), nil
	}
	t, err := time.Parse(time.RFC3339Nano, cfg.Value)
	if err != nil {
		// Fallback to bootstrap window if the saved value is malformed.
		return time.Now().Add(-1 * time.Hour), nil
	}
	return t, nil
}

// Save persists the high-water mark.
func (c *Cursor) Save(ctx context.Context, t time.Time) error {
	return c.store.Set(ctx, model.SystemConfigKeyLinearLastSync, t.UTC().Format(time.RFC3339Nano))
}
