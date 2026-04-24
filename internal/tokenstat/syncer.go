package tokenstat

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"time"

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
}

func NewSyncer(db *sql.DB, tokenStore *store.TokenStatsStore) *Syncer {
	home, _ := os.UserHomeDir()
	mappingDir := filepath.Join(home, ".openbee", ".codex", "sessions")
	return &Syncer{
		db:         db,
		tokenStore: tokenStore,
		parsers: map[string]Parser{
			"claude": NewClaudeParser(),
			"codex":  NewCodexParser(mappingDir),
			"pi":     NewPiParser(),
		},
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
	empty, err := s.tokenStore.IsEmpty()
	if err != nil {
		return nil, err
	}

	var (
		rows  *sql.Rows
		query string
		args  []any
	)
	if empty {
		query = `
			SELECT DISTINCT e.session_id, w.engine
			FROM bee_executions e
			JOIN bee_workers w ON w.id = e.worker_id
			WHERE w.engine IN ('claude', 'codex', 'pi') AND e.worker_id IS NOT NULL`
	} else {
		since := time.Now().AddDate(0, 0, -incrementalDays).UnixMilli()
		query = `
			SELECT DISTINCT e.session_id, w.engine
			FROM bee_executions e
			JOIN bee_workers w ON w.id = e.worker_id
			WHERE w.engine IN ('claude', 'codex', 'pi')
			  AND e.worker_id IS NOT NULL
			  AND e.completed_at > ?`
		args = []any{since}
	}

	rows, err = s.db.QueryContext(ctx, query, args...)
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
	parser, ok := s.parsers[engine]
	if !ok {
		return nil
	}
	usages, err := parser.Parse(sessionID)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for _, u := range usages {
		if err := s.tokenStore.Upsert(model.TokenStats{
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
		}
	}
	return nil
}
