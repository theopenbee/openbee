package store

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type dbExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

type TokenStatsStore struct {
	db *sql.DB
}

func NewTokenStatsStore(db *sql.DB) *TokenStatsStore {
	return &TokenStatsStore{db: db}
}

func (s *TokenStatsStore) IsEmpty() (bool, error) {
	var exists int
	if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM bee_token_stats)`).Scan(&exists); err != nil {
		return false, fmt.Errorf("check token stats empty: %w", err)
	}
	return exists == 0, nil
}

func (s *TokenStatsStore) Upsert(stat model.TokenStats) error {
	return upsertTokenStat(s.db, stat)
}

func (s *TokenStatsStore) UpsertTx(tx *sql.Tx, stat model.TokenStats) error {
	return upsertTokenStat(tx, stat)
}

func upsertTokenStat(db dbExecer, stat model.TokenStats) error {
	if stat.ID == "" {
		stat.ID = uuid.New().String()
	}
	_, err := db.Exec(
		`INSERT INTO bee_token_stats
		     (id, session_id, agent_type, model, input_tokens, output_tokens,
		      cache_creation_tokens, cache_read_tokens, total_tokens, synced_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(session_id, model) DO UPDATE SET
		     agent_type            = excluded.agent_type,
		     input_tokens          = excluded.input_tokens,
		     output_tokens         = excluded.output_tokens,
		     cache_creation_tokens = excluded.cache_creation_tokens,
		     cache_read_tokens     = excluded.cache_read_tokens, total_tokens = excluded.total_tokens,
		     synced_at             = excluded.synced_at`,
		stat.ID, stat.SessionID, stat.AgentType, stat.Model,
		stat.InputTokens, stat.OutputTokens,
		stat.CacheCreationTokens, stat.CacheReadTokens, stat.TotalTokens,
		stat.SyncedAt,
	)
	return err
}

func (s *TokenStatsStore) GetBySessionID(sessionID string) ([]model.TokenStats, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, agent_type, model, input_tokens, output_tokens,
		        cache_creation_tokens, cache_read_tokens, total_tokens, synced_at
		 FROM bee_token_stats WHERE session_id = ?`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query token stats by session: %w", err)
	}
	defer rows.Close()
	var stats []model.TokenStats
	for rows.Next() {
		var st model.TokenStats
		if err := rows.Scan(
			&st.ID, &st.SessionID, &st.AgentType, &st.Model,
			&st.InputTokens, &st.OutputTokens,
			&st.CacheCreationTokens, &st.CacheReadTokens,
			&st.TotalTokens, &st.SyncedAt,
		); err != nil {
			return nil, fmt.Errorf("scan token stats: %w", err)
		}
		stats = append(stats, st)
	}
	return stats, rows.Err()
}
