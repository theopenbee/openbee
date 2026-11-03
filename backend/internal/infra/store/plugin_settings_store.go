package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/theopenbee/openbee/internal/infra/model"
)

type PluginSettingsStore struct {
	db *sql.DB
}

func NewPluginSettingsStore(db *sql.DB) *PluginSettingsStore {
	return &PluginSettingsStore{db: db}
}

func (s *PluginSettingsStore) Get(ctx context.Context, id string) (model.PluginSettings, bool, error) {
	var p model.PluginSettings
	err := s.db.QueryRowContext(ctx,
		`SELECT id, enabled, bot_name, config, updated_at FROM bee_plugin_settings WHERE id = ?`, id,
	).Scan(&p.ID, &p.Enabled, &p.BotName, &p.Config, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.PluginSettings{}, false, nil
	}
	if err != nil {
		return model.PluginSettings{}, false, fmt.Errorf("get plugin settings %q: %w", id, err)
	}
	return p, true, nil
}

func (s *PluginSettingsStore) List(ctx context.Context) ([]model.PluginSettings, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, enabled, bot_name, config, updated_at FROM bee_plugin_settings ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list plugin settings: %w", err)
	}
	defer rows.Close()
	var out []model.PluginSettings
	for rows.Next() {
		var p model.PluginSettings
		if err := rows.Scan(&p.ID, &p.Enabled, &p.BotName, &p.Config, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan plugin settings: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PluginSettingsStore) Upsert(ctx context.Context, p model.PluginSettings) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO bee_plugin_settings (id, enabled, bot_name, config, updated_at) VALUES (?, ?, ?, ?, ?)
         ON CONFLICT(id) DO UPDATE SET enabled = excluded.enabled, bot_name = excluded.bot_name,
         config = excluded.config, updated_at = excluded.updated_at`,
		p.ID, p.Enabled, p.BotName, p.Config, time.Now().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("save plugin settings %q: %w", p.ID, err)
	}
	return nil
}

func (s *PluginSettingsStore) Delete(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM bee_plugin_settings WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete plugin settings %q: %w", id, err)
	}
	return nil
}
