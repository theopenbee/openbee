package store

import (
	"database/sql"
	"fmt"

	"github.com/theopenbee/openbee/internal/infra/model"
)

type UsageStore struct {
	db *sql.DB
}

func NewUsageStore(db *sql.DB) *UsageStore {
	return &UsageStore{db: db}
}

const usageSelect = `SELECT id, execution_id, model, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, total_tokens, cost_usd, synced_at FROM bee_usage_records`

func scanUsageRecord(scanner interface{ Scan(...any) error }) (model.UsageRecord, error) {
	var r model.UsageRecord
	err := scanner.Scan(&r.ID, &r.ExecutionID, &r.Model, &r.InputTokens, &r.OutputTokens,
		&r.CacheCreationTokens, &r.CacheReadTokens, &r.TotalTokens, &r.CostUSD, &r.SyncedAt)
	return r, err
}

// INSERT OR IGNORE makes duplicate execution_id calls safe (idempotent).
func (s *UsageStore) Insert(record *model.UsageRecord) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO bee_usage_records
         (id, execution_id, model, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, total_tokens, cost_usd, synced_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID, record.ExecutionID, record.Model,
		record.InputTokens, record.OutputTokens,
		record.CacheCreationTokens, record.CacheReadTokens,
		record.TotalTokens, record.CostUSD, record.SyncedAt,
	)
	if err != nil {
		return fmt.Errorf("insert usage record: %w", err)
	}
	return nil
}

func (s *UsageStore) GetByExecutionID(executionID string) (*model.UsageRecord, error) {
	row := s.db.QueryRow(usageSelect+` WHERE execution_id = ?`, executionID)
	r, err := scanUsageRecord(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get usage record: %w", err)
	}
	return &r, nil
}

func (s *UsageStore) ListUnsynced(limit int) ([]model.UnsyncedExecution, error) {
	rows, err := s.db.Query(
		`SELECT e.id, e.log_path
         FROM bee_executions e
         LEFT JOIN bee_usage_records u ON e.id = u.execution_id
         WHERE e.status IN (?, ?)
           AND e.log_path != ''
           AND u.id IS NULL
         LIMIT ?`, model.ExecStatusCompleted, model.ExecStatusFailed, limit)
	if err != nil {
		return nil, fmt.Errorf("list unsynced executions: %w", err)
	}
	defer rows.Close()

	result := make([]model.UnsyncedExecution, 0, limit)
	for rows.Next() {
		var e model.UnsyncedExecution
		if err := rows.Scan(&e.ID, &e.LogPath); err != nil {
			return nil, fmt.Errorf("scan unsynced execution: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
