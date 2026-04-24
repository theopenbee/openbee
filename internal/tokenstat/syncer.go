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

const syncInterval = 10 * time.Minute

// tombstoneModel is stored when all parsers fail for a session so the syncer
// stops retrying it (synced_at advances past completed_at).
const tombstoneModel = "unknown"

var defaultParserOrder = []string{ai.EngineClaude, ai.EngineCodex, ai.EnginePi, ai.EngineKimi}

type Syncer struct {
	db         *sql.DB
	tokenStore *store.TokenStatsStore
	parsers    map[string]Parser
}

func NewSyncer(db *sql.DB, tokenStore *store.TokenStatsStore) *Syncer {
	return &Syncer{
		db:         db,
		tokenStore: tokenStore,
		parsers: map[string]Parser{
			ai.EngineClaude: NewClaudeParser(),
			ai.EngineCodex:  NewCodexParser(config.DefaultCodexSessionsDir()),
			ai.EnginePi:     NewPiParser(),
			ai.EngineKimi:   NewKimiParser(),
		},
	}
}

func (s *Syncer) Run(ctx context.Context) {
	logger.Info("tokenstat: sync loop started", zap.Duration("interval", syncInterval))
	s.SyncOnce(ctx)
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("tokenstat: sync loop stopped")
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
	if len(sessions) == 0 {
		logger.Debug("tokenstat: no sessions pending sync")
		return
	}
	logger.Info("tokenstat: syncing sessions", zap.Int("count", len(sessions)))
	var synced, failed int
	for _, item := range sessions {
		if err := s.syncSession(item.sessionID, item.engine); err != nil {
			failed++
			logger.Warn("tokenstat: sync session failed",
				zap.String("session_id", item.sessionID),
				zap.String("engine", item.engine),
				zap.Error(err))
		} else {
			synced++
		}
	}
	logger.Info("tokenstat: sync round complete",
		zap.Int("synced", synced),
		zap.Int("failed", failed))
}

type sessionItem struct {
	sessionID string
	engine    string
}

func (s *Syncer) collectSessions(ctx context.Context) ([]sessionItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.session_id, COALESCE(MAX(NULLIF(e.engine, '')), '')
		FROM bee_executions e
		LEFT JOIN bee_token_stats ts ON ts.session_id = e.session_id
		WHERE (e.engine = '' OR e.engine IN (?, ?, ?, ?))
		GROUP BY e.session_id
		HAVING MAX(e.completed_at) > COALESCE(MAX(ts.synced_at), 0)`,
		ai.EngineClaude, ai.EngineCodex, ai.EnginePi, ai.EngineKimi)
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
				logger.Debug("tokenstat: session data not found",
					zap.String("session_id", sessionID),
					zap.String("parser", parserName))
				continue
			}
			logger.Warn("tokenstat: parser error",
				zap.String("session_id", sessionID),
				zap.String("parser", parserName),
				zap.Error(err))
			if firstErr == nil {
				firstErr = fmt.Errorf("%s parser: %w", parserName, err)
			}
			continue
		}
		if err := s.storeUsages(usages); err != nil {
			return fmt.Errorf("store usages: %w", err)
		}
		logger.Info("tokenstat: session synced",
			zap.String("session_id", sessionID),
			zap.String("parser", parserName),
			zap.Int("models", len(usages)))
		return nil
	}
	if firstErr != nil {
		return firstErr
	}
	if engine == "" {
		logger.Debug("tokenstat: legacy session has no data, writing tombstone",
			zap.String("session_id", sessionID))
	} else {
		logger.Warn("tokenstat: no token data found for session, writing tombstone",
			zap.String("session_id", sessionID),
			zap.String("engine", engine))
	}
	return s.storeUsages([]SessionTokenUsage{{SessionID: sessionID, Model: tombstoneModel}})
}

func (s *Syncer) parserOrder(preferred string) []string {
	if _, ok := s.parsers[preferred]; preferred == "" || !ok {
		return defaultParserOrder
	}
	order := []string{preferred}
	for _, name := range defaultParserOrder {
		if name != preferred {
			order = append(order, name)
		}
	}
	return order
}

func (s *Syncer) storeUsages(usages []SessionTokenUsage) error {
	if len(usages) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	for _, u := range usages {
		if err := s.tokenStore.UpsertTx(tx, model.TokenStats{
			SessionID:           u.SessionID,
			AgentType:           u.AgentType,
			Model:               u.Model,
			InputTokens:         u.InputTokens,
			OutputTokens:        u.OutputTokens,
			CacheCreationTokens: u.CacheCreationTokens,
			CacheReadTokens:     u.CacheReadTokens,
			TotalTokens:         u.InputTokens + u.OutputTokens + u.CacheCreationTokens + u.CacheReadTokens,
			SyncedAt:            now,
		}); err != nil {
			return fmt.Errorf("upsert: %w", err)
		}
	}
	return tx.Commit()
}
