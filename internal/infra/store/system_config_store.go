package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/theopenbee/openbee/internal/infra/model"
)

// SystemConfigStore reads and writes global system configuration.
type SystemConfigStore struct {
	db *sql.DB
}

// NewSystemConfigStore constructs a SystemConfigStore.
func NewSystemConfigStore(db *sql.DB) *SystemConfigStore {
	return &SystemConfigStore{db: db}
}

// Get retrieves a system config by key. Returns (config, true, nil) if found,
// (zero, false, nil) if not found, or (zero, false, err) on error.
func (s *SystemConfigStore) Get(ctx context.Context, key string) (model.SystemConfig, bool, error) {
	var cfg model.SystemConfig
	err := s.db.QueryRowContext(ctx,
		`SELECT key, value, updated_at FROM bee_system_configs WHERE key = ?`, key,
	).Scan(&cfg.Key, &cfg.Value, &cfg.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.SystemConfig{}, false, nil
	}
	if err != nil {
		return model.SystemConfig{}, false, fmt.Errorf("get system config %q: %w", key, err)
	}
	return cfg, true, nil
}

// Set upserts a system config entry.
func (s *SystemConfigStore) Set(ctx context.Context, key, value string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO bee_system_configs (key, value, updated_at) VALUES (?, ?, ?)
         ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, now,
	)
	if err != nil {
		return fmt.Errorf("set system config %q: %w", key, err)
	}
	return nil
}
