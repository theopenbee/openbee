package tokenstat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/theopenbee/openbee/internal/ai/bridge"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"go.uber.org/zap"
)

const syncInterval = 10 * time.Minute

// usageBridge is the subset of bridge.Bridge that Syncer needs. Declared
// locally so tests can substitute a lightweight fake without importing the
// full bridge package.
type usageBridge interface {
	AllEngines() []string
	IsEnabled(name string) bool
	CollectUsage(ctx context.Context, engineName, sessionID string) ([]bridge.Usage, error)
}

// Syncer periodically reads completed sessions from bee_executions and asks
// the matching engine adapter to produce per-model token usage, then upserts
// into bee_token_stats. Engines whose bee_executions.engine field is empty
// (legacy data) are dispatched through a fixed fallback chain.
type Syncer struct {
	db         *sql.DB
	tokenStore *store.TokenStatsStore
	br         usageBridge
	collectSQL string
}

// NewSyncer builds a Syncer that dispatches through the supplied bridge.
func NewSyncer(db *sql.DB, tokenStore *store.TokenStatsStore, br usageBridge) *Syncer {
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

// syncSession dispatches one session to the appropriate engine. A known engine
// is tried once; an empty/unregistered engine walks the fallback chain.
func (s *Syncer) syncSession(ctx context.Context, sessionID, engine string) error {
	if engine != "" {
		if s.br.IsEnabled(engine) {
			err := s.tryEngine(ctx, sessionID, engine)
			if errors.Is(err, bridge.ErrSessionDataNotFound) {
				return s.tombstone(sessionID, "known engine: session data not found")
			}
			return err
		}
	}

	// Empty or unknown engine → fallback chain.
	var sawNotFound bool
	for _, name := range s.br.AllEngines() {
		if !s.br.IsEnabled(name) {
			continue
		}
		err := s.tryEngine(ctx, sessionID, name)
		if err == nil || !errors.Is(err, bridge.ErrSessionDataNotFound) {
			return err
		}
		sawNotFound = true
	}
	if sawNotFound {
		return s.tombstone(sessionID, "no adapter found data (legacy fallback)")
	}
	return s.tombstone(sessionID, "no adapters available")
}

func (s *Syncer) tryEngine(ctx context.Context, sessionID, engine string) error {
	usages, err := s.br.CollectUsage(ctx, engine, sessionID)
	if err != nil {
		if errors.Is(err, bridge.ErrSessionDataNotFound) {
			logger.Debug("tokenstat: session data not found",
				zap.String("session_id", sessionID),
				zap.String("engine", engine))
			return err
		}
		return fmt.Errorf("%s collector: %w", engine, err)
	}
	if len(usages) == 0 {
		logger.Debug("tokenstat: session located but empty, writing tombstone",
			zap.String("session_id", sessionID),
			zap.String("engine", engine))
		return s.tombstone(sessionID, "empty usages")
	}
	if err := s.upsertRows(sessionID, engine, usages); err != nil {
		return fmt.Errorf("store usages: %w", err)
	}
	logger.Info("tokenstat: session synced",
		zap.String("session_id", sessionID),
		zap.String("engine", engine),
		zap.Int("models", len(usages)))
	return nil
}

func (s *Syncer) tombstone(sessionID, reason string) error {
	logger.Debug("tokenstat: tombstoning session",
		zap.String("session_id", sessionID),
		zap.String("reason", reason))
	return s.upsertRows(sessionID, "", []bridge.Usage{{Model: store.TombstoneModel}})
}

func (s *Syncer) upsertRows(sessionID, agentType string, usages []bridge.Usage) error {
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
