package tokenstat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/theopenbee/openbee/internal/bridge"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"go.uber.org/zap"
)

const syncInterval = 10 * time.Minute

// Syncer periodically reads completed sessions from bee_executions and asks
// bridge to produce per-model token usage, then upserts into bee_token_stats.
type Syncer struct {
	db         *sql.DB
	tokenStore *store.TokenStatsStore
	br         bridge.Bridge
	collectSQL string
}

// NewSyncer builds a Syncer that dispatches token usage collection through bridge.
func NewSyncer(db *sql.DB, tokenStore *store.TokenStatsStore, br bridge.Bridge) *Syncer {
	collectSQL := `
		SELECT e.session_id, COALESCE(MAX(NULLIF(e.engine, '')), '')
		FROM bee_executions e
		LEFT JOIN bee_token_stats ts ON ts.session_id = e.session_id
		GROUP BY e.session_id
		HAVING MAX(e.completed_at) > COALESCE(MAX(ts.synced_at), 0)
		LIMIT 500`
	return &Syncer{
		db:         db,
		tokenStore: tokenStore,
		br:         br,
		collectSQL: collectSQL,
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
		if err := s.syncSession(ctx, item.sessionID, item.engine); err != nil {
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
	rows, err := s.db.QueryContext(ctx, s.collectSQL)
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

// syncSession dispatches one session through bridge. Bridge owns any legacy
// fallback behavior for empty or unrecognized engine names.
func (s *Syncer) syncSession(ctx context.Context, sessionID, engine string) error {
	result, err := s.br.CollectTokenUsage(ctx, sessionID, engine)
	if err != nil {
		if errors.Is(err, bridge.ErrSessionDataNotFound) {
			logger.Debug("tokenstat: session data not found",
				zap.String("session_id", sessionID),
				zap.String("engine", engine))
			return s.tombstone(sessionID, "session data not found")
		}
		return fmt.Errorf("%s collector: %w", engine, err)
	}
	if len(result.Usages) == 0 {
		logger.Debug("tokenstat: session located but empty, writing tombstone",
			zap.String("session_id", sessionID),
			zap.String("engine", result.Engine))
		return s.tombstone(sessionID, "empty usages")
	}
	if err := s.upsertRows(sessionID, result.Engine, result.Usages); err != nil {
		return fmt.Errorf("store usages: %w", err)
	}
	logger.Info("tokenstat: session synced",
		zap.String("session_id", sessionID),
		zap.String("engine", result.Engine),
		zap.Int("models", len(result.Usages)))
	return nil
}

func (s *Syncer) tombstone(sessionID, reason string) error {
	logger.Debug("tokenstat: tombstoning session",
		zap.String("session_id", sessionID),
		zap.String("reason", reason))
	return s.upsertRows(sessionID, "", []bridge.TokenUsage{{Model: store.TombstoneModel}})
}

func (s *Syncer) upsertRows(sessionID, agentType string, usages []bridge.TokenUsage) error {
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
			SessionID:           sessionID,
			AgentType:           agentType,
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
