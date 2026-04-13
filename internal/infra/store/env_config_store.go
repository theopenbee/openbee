package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type EnvConfigStore struct {
	db *sql.DB
}

func NewEnvConfigStore(db *sql.DB) *EnvConfigStore {
	return &EnvConfigStore{db: db}
}

const envConfigColumns = `id, scope, scope_id, key, enc_value, masked, created_at, updated_at`

func scanEnvConfig(scanner interface{ Scan(...any) error }) (*model.EnvConfig, error) {
	var cfg model.EnvConfig
	err := scanner.Scan(
		&cfg.ID, &cfg.Scope, &cfg.ScopeID, &cfg.Key,
		&cfg.EncValue, &cfg.Masked, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func scanEnvConfigs(rows *sql.Rows) ([]*model.EnvConfig, error) {
	var cfgs []*model.EnvConfig
	for rows.Next() {
		cfg, err := scanEnvConfig(rows)
		if err != nil {
			return nil, err
		}
		cfgs = append(cfgs, cfg)
	}
	return cfgs, rows.Err()
}

func (s *EnvConfigStore) Create(cfg *model.EnvConfig) error {
	cfg.ID = uuid.New().String()
	cfg.CreatedAt = time.Now().UnixMilli()
	cfg.UpdatedAt = cfg.CreatedAt

	_, err := s.db.Exec(
		`INSERT INTO bee_env_configs (id, scope, scope_id, key, enc_value, masked, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		cfg.ID, cfg.Scope, cfg.ScopeID, cfg.Key,
		cfg.EncValue, cfg.Masked, cfg.CreatedAt, cfg.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert env config: %w", err)
	}
	return nil
}

func (s *EnvConfigStore) List(scope string, scopeID *string) ([]*model.EnvConfig, error) {
	var rows *sql.Rows
	var err error
	if scopeID == nil {
		rows, err = s.db.Query(
			`SELECT `+envConfigColumns+` FROM bee_env_configs WHERE scope = ? AND scope_id IS NULL ORDER BY created_at ASC`,
			scope,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT `+envConfigColumns+` FROM bee_env_configs WHERE scope = ? AND scope_id = ? ORDER BY created_at ASC`,
			scope, *scopeID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list env configs: %w", err)
	}
	defer rows.Close()
	return scanEnvConfigs(rows)
}

func (s *EnvConfigStore) Get(id string) (*model.EnvConfig, error) {
	row := s.db.QueryRow(`SELECT `+envConfigColumns+` FROM bee_env_configs WHERE id = ?`, id)
	cfg, err := scanEnvConfig(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get env config: %w", err)
	}
	return cfg, nil
}

func (s *EnvConfigStore) Update(id, encValue, masked string) error {
	_, err := s.db.Exec(
		`UPDATE bee_env_configs SET enc_value = ?, masked = ?, updated_at = ? WHERE id = ?`,
		encValue, masked, time.Now().UnixMilli(), id,
	)
	if err != nil {
		return fmt.Errorf("update env config: %w", err)
	}
	return nil
}

func (s *EnvConfigStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM bee_env_configs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete env config: %w", err)
	}
	return nil
}

// ListForDepartments returns department-scoped env configs for the given department IDs.
// Returns empty result without querying if departmentIDs is empty.
func (s *EnvConfigStore) ListForDepartments(departmentIDs []string) ([]*model.EnvConfig, error) {
	if len(departmentIDs) == 0 {
		return []*model.EnvConfig{}, nil
	}
	args := append([]any{"department"}, stringsToArgs(departmentIDs)...)
	rows, err := s.db.Query(
		`SELECT `+envConfigColumns+` FROM bee_env_configs
		 WHERE scope = ? AND scope_id IN (`+inPlaceholders(len(departmentIDs))+`)
		 ORDER BY scope_id ASC`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list env configs for departments: %w", err)
	}
	defer rows.Close()
	return scanEnvConfigs(rows)
}
