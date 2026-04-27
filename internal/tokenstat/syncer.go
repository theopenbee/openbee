package tokenstat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"go.uber.org/zap"
)

const syncInterval = 10 * time.Minute

// Syncer periodically reads completed sessions from bee_executions and asks
// the matching engine adapter to produce per-model token usage, then upserts
// into bee_token_stats. Engines whose bee_executions.engine field is empty
// (legacy data) are dispatched through a fixed fallback chain.
type Syncer struct {
	db         *sql.DB
	tokenStore *store.TokenStatsStore

	// adapters maps engine name → adapter. Always non-empty for non-legacy rows.
	adapters map[string]ai.EngineAdapter

	// fallbackOrder is the deterministic engine name order used when
	// dispatching a session whose engine field is empty. Each name must
	// appear in adapters; absent names are silently skipped.
	fallbackOrder []string

	// engines, collectSQL, engineArgs are precomputed for collectSessions.
	engines    []string
	collectSQL string
	engineArgs []any
}

// NewSyncer builds a Syncer that dispatches to the supplied adapters.
// fallbackOrder controls the legacy fallback chain — pass ai.AllEngines()
// to preserve the historical chain order.
func NewSyncer(db *sql.DB, tokenStore *store.TokenStatsStore, adapters map[string]ai.EngineAdapter, fallbackOrder []string) *Syncer {
	engines := ai.AllEngines()
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(engines)), ",")
	collectSQL := fmt.Sprintf(`
		SELECT e.session_id, COALESCE(MAX(NULLIF(e.engine, '')), '')
		FROM bee_executions e
		LEFT JOIN bee_token_stats ts ON ts.session_id = e.session_id
		WHERE (e.engine = '' OR e.engine IN (%s))
		GROUP BY e.session_id
		HAVING MAX(e.completed_at) > COALESCE(MAX(ts.synced_at), 0)
		LIMIT 500`, placeholders)
	engineArgs := make([]any, len(engines))
	for i, e := range engines {
		engineArgs[i] = e
	}
	return &Syncer{
		db:            db,
		tokenStore:    tokenStore,
		adapters:      adapters,
		fallbackOrder: fallbackOrder,
		engines:       engines,
		collectSQL:    collectSQL,
		engineArgs:    engineArgs,
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
	rows, err := s.db.QueryContext(ctx, s.collectSQL, s.engineArgs...)
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

// syncSession dispatches one session to the appropriate adapter.
//   - If item.engine is non-empty and registered: call that adapter once.
//   - If item.engine is empty OR not registered (legacy data, or an engine
//     that's no longer compiled in): walk fallbackOrder, advancing only
//     when the adapter reports ErrSessionDataNotFound.
func (s *Syncer) syncSession(ctx context.Context, sessionID, engine string) error {
	if engine != "" {
		if adapter, ok := s.adapters[engine]; ok {
			return s.tryAdapter(ctx, sessionID, engine, adapter)
		}
	}

	// Empty or unknown engine → fallback chain.
	var sawNotFound bool
	for _, name := range s.fallbackOrder {
		adapter, ok := s.adapters[name]
		if !ok {
			continue
		}
		err := s.tryAdapter(ctx, sessionID, name, adapter)
		if err == nil || !errors.Is(err, ai.ErrSessionDataNotFound) {
			return err
		}
		sawNotFound = true
	}
	if sawNotFound {
		return s.tombstone(sessionID, "no adapter found data (legacy fallback)")
	}
	return s.tombstone(sessionID, "no adapters available")
}

// tryAdapter calls the adapter's CollectTokenUsage and persists the result.
//   - usages non-empty → upsert and return nil
//   - usages empty + err == nil → tombstone and return nil
//   - err == ErrSessionDataNotFound → propagate so the caller can fall through
//   - other err → propagate (the caller logs and counts as failed)
func (s *Syncer) tryAdapter(ctx context.Context, sessionID, engine string, adapter ai.EngineAdapter) error {
	usages, err := adapter.CollectTokenUsage(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ai.ErrSessionDataNotFound) {
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
	if err := s.storeUsages(sessionID, engine, usages); err != nil {
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
	return s.upsertRows(sessionID, "", []ai.TokenUsage{{Model: store.TombstoneModel}})
}

func (s *Syncer) storeUsages(sessionID, engine string, usages []ai.TokenUsage) error {
	return s.upsertRows(sessionID, engine, usages)
}

func (s *Syncer) upsertRows(sessionID, agentType string, usages []ai.TokenUsage) error {
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
