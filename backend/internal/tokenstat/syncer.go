package tokenstat

import (
	"context"
	"database/sql"
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

type Syncer struct {
	db         *sql.DB
	tokenStore *store.TokenStatsStore
	src        ai.EngineSource
}

func NewSyncer(db *sql.DB, tokenStore *store.TokenStatsStore, src ai.EngineSource) *Syncer {
	return &Syncer{db: db, tokenStore: tokenStore, src: src}
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
	ids := ai.EngineIDs(s.src)
	if len(ids) == 0 {
		logger.Debug("tokenstat: no engines, skipping sync")
		return
	}
	byEngine, count, err := s.collectSessions(ctx, ids)
	if err != nil {
		logger.Error("tokenstat: collect sessions", zap.Error(err))
		return
	}
	if count == 0 {
		logger.Debug("tokenstat: no sessions pending sync")
		return
	}
	logger.Info("tokenstat: syncing sessions", zap.Int("count", count))
	for _, id := range ids {
		if sessions := byEngine[id]; len(sessions) > 0 {
			s.syncEngine(ctx, id, sessions)
		}
	}
	if legacy := byEngine[""]; len(legacy) > 0 {
		s.syncLegacy(ctx, ids, legacy)
	}
}

func (s *Syncer) collectSessions(ctx context.Context, engineIDs []string) (map[string][]string, int, error) {
	args := make([]any, len(engineIDs))
	for i, id := range engineIDs {
		args[i] = id
	}
	query := `
		SELECT e.session_id, COALESCE(MAX(NULLIF(e.engine, '')), '')
		FROM bee_executions e
		LEFT JOIN bee_token_stats ts ON ts.session_id = e.session_id
		GROUP BY e.session_id
		HAVING MAX(e.completed_at) > COALESCE(MAX(ts.synced_at), 0)
			AND COALESCE(MAX(NULLIF(e.engine, '')), '') IN (''` + strings.Repeat(", ?", len(engineIDs)) + `)
		LIMIT 500`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	byEngine := make(map[string][]string)
	count := 0
	for rows.Next() {
		var sessionID, engine string
		if err := rows.Scan(&sessionID, &engine); err != nil {
			return nil, 0, err
		}
		byEngine[engine] = append(byEngine[engine], sessionID)
		count++
	}
	return byEngine, count, rows.Err()
}

func (s *Syncer) syncEngine(ctx context.Context, engine string, sessionIDs []string) {
	found, ok := s.collect(ctx, engine, sessionIDs)
	if !ok {
		return
	}
	for _, sessionID := range sessionIDs {
		s.save(sessionID, engine, found[sessionID])
	}
}

func (s *Syncer) syncLegacy(ctx context.Context, engineIDs, sessionIDs []string) {
	remaining := sessionIDs
	for _, engine := range engineIDs {
		if len(remaining) == 0 {
			return
		}
		found, ok := s.collect(ctx, engine, remaining)
		if !ok {
			return
		}
		var rest []string
		for _, sessionID := range remaining {
			if usages, hit := found[sessionID]; hit {
				s.save(sessionID, engine, usages)
			} else {
				rest = append(rest, sessionID)
			}
		}
		remaining = rest
	}
	for _, sessionID := range remaining {
		s.save(sessionID, "", nil)
	}
}

func (s *Syncer) collect(ctx context.Context, engine string, sessionIDs []string) (map[string][]ai.TokenUsage, bool) {
	adapter, ok := s.src.Engine(engine)
	if !ok {
		return nil, false
	}
	found, err := adapter.CollectTokenUsage(ctx, sessionIDs)
	if err != nil {
		logger.Warn("tokenstat: collect token usage failed",
			zap.String("engine", engine),
			zap.Int("sessions", len(sessionIDs)),
			zap.Error(err))
		return nil, false
	}
	return found, true
}

func (s *Syncer) save(sessionID, engine string, usages []ai.TokenUsage) {
	var err error
	if len(usages) == 0 {
		err = s.tombstone(sessionID)
	} else {
		err = s.upsertRows(sessionID, engine, usages)
	}
	if err != nil {
		logger.Warn("tokenstat: store session usage failed",
			zap.String("session_id", sessionID),
			zap.String("engine", engine),
			zap.Error(err))
	}
}

func (s *Syncer) tombstone(sessionID string) error {
	logger.Debug("tokenstat: tombstoning session", zap.String("session_id", sessionID))
	return s.upsertRows(sessionID, "", []ai.TokenUsage{{Model: store.TombstoneModel}})
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
