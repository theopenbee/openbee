package tokenstat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"go.uber.org/zap"
)

const (
	syncInterval    = 10 * time.Minute
	incrementalDays = 30
)

type Syncer struct {
	db         *sql.DB
	tokenStore *store.TokenStatsStore
	parsers    map[string]Parser
	parserList []string
}

func NewSyncer(db *sql.DB, tokenStore *store.TokenStatsStore) *Syncer {
	return &Syncer{
		db:         db,
		tokenStore: tokenStore,
		parsers: map[string]Parser{
			ai.EngineClaude: NewClaudeParser(),
			ai.EngineCodex:  NewCodexParser(config.DefaultCodexSessionsDir()),
			ai.EnginePi:     NewPiParser(),
		},
		parserList: []string{ai.EngineClaude, ai.EngineCodex, ai.EnginePi},
	}
}

func (s *Syncer) Run(ctx context.Context) {
	s.SyncOnce(ctx)
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.SyncOnce(ctx)
		}
	}
}

func (s *Syncer) SyncOnce(ctx context.Context) {
	sessions, err := s.collectSessions(ctx)
	if err != nil {
		logger.Error("tokenstat: collect sessions", zap.Error(err))
		return
	}
	for _, item := range sessions {
		if err := s.syncSession(item.sessionID, item.engine); err != nil {
			logger.Warn("tokenstat: sync session",
				zap.String("session_id", item.sessionID),
				zap.String("engine", item.engine),
				zap.Error(err))
		}
	}
}

type sessionItem struct {
	sessionID string
	engine    string
}

func (s *Syncer) collectSessions(ctx context.Context) ([]sessionItem, error) {
	since := time.Now().AddDate(0, 0, -incrementalDays).UnixMilli()
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.session_id, MAX(e.engine)
		FROM bee_executions e
		WHERE e.worker_id IS NOT NULL
		  AND (e.engine = '' OR e.engine IN (?, ?, ?))
		GROUP BY e.session_id
		HAVING MAX(CASE WHEN e.completed_at > ? THEN 1 ELSE 0 END) = 1
		    OR NOT EXISTS (
		        SELECT 1 FROM bee_token_stats ts
		        WHERE ts.session_id = e.session_id
		    )`, ai.EngineClaude, ai.EngineCodex, ai.EnginePi, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []sessionItem
	for rows.Next() {
		var item sessionItem
		if err := rows.Scan(&item.sessionID, &item.engine); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Syncer) syncSession(sessionID, engine string) error {
	var firstErr error
	for _, parserName := range s.parserOrder(engine) {
		parser := s.parsers[parserName]
		usages, err := parser.Parse(sessionID)
		if err != nil {
			if errors.Is(err, ErrSessionDataNotFound) {
				continue
			}
			if firstErr == nil {
				firstErr = fmt.Errorf("%s parser: %w", parserName, err)
			}
			continue
		}
		s.storeUsages(sessionID, usages)
		return nil
	}
	if firstErr != nil {
		return firstErr
	}
	// All parsers returned ErrSessionDataNotFound.
	// For legacy executions without an engine hint, missing data is expected.
	if engine == "" {
		return nil
	}
	return fmt.Errorf("no token session data found for %s", sessionID)
}

func (s *Syncer) parserOrder(preferred string) []string {
	if preferred == "" {
		return append([]string(nil), s.parserList...)
	}
	if _, ok := s.parsers[preferred]; !ok {
		return append([]string(nil), s.parserList...)
	}
	order := []string{preferred}
	for _, name := range s.parserList {
		if name != preferred {
			order = append(order, name)
		}
	}
	return order
}

func (s *Syncer) storeUsages(sessionID string, usages []SessionTokenUsage) {
	now := time.Now().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		logger.Error("tokenstat: begin transaction", zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	for _, u := range usages {
		if err := s.tokenStore.UpsertTx(tx, model.TokenStats{
			SessionID:           u.SessionID,
			AgentType:           u.AgentType,
			Model:               u.Model,
			InputTokens:         u.InputTokens,
			OutputTokens:        u.OutputTokens,
			CacheCreationTokens: u.CacheCreationTokens,
			CacheReadTokens:     u.CacheReadTokens,
			SyncedAt:            now,
		}); err != nil {
			logger.Error("tokenstat: upsert",
				zap.String("session_id", sessionID),
				zap.Error(err))
			_ = tx.Rollback()
			return
		}
	}
	if err := tx.Commit(); err != nil {
		logger.Error("tokenstat: commit transaction", zap.String("session_id", sessionID), zap.Error(err))
	}
}
