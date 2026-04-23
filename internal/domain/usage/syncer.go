package usage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	usageparser "github.com/theopenbee/openbee/internal/ai/usage"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
)

var log = logger.With(zap.String("component", "usagesyncer"))

type usageSyncStore interface {
	ListUnsynced(limit int) ([]model.UnsyncedExecution, error)
	Insert(record *model.UsageRecord) error
}

// SyncerConfig holds filesystem paths needed to locate engine session files.
type SyncerConfig struct {
	PiSessionsDir    string // e.g. ~/.openbee/.pi/sessions
	CodexStoreDir    string // e.g. ~/.openbee/.codex/sessions (uuid→thread_id mapping files)
	CodexSessionsDir string // codex native sessions dir, e.g. ~/.codex/sessions
}

type UsageSyncer struct {
	store     usageSyncStore
	cfg       SyncerConfig
	interval  time.Duration
	batchSize int
}

func NewUsageSyncer(store usageSyncStore, interval time.Duration, batchSize int, cfg SyncerConfig) *UsageSyncer {
	return &UsageSyncer{store: store, cfg: cfg, interval: interval, batchSize: batchSize}
}

func (s *UsageSyncer) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			for s.syncBatch() {
				if ctx.Err() != nil {
					return
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *UsageSyncer) syncBatch() bool {
	execs, err := s.store.ListUnsynced(s.batchSize)
	if err != nil {
		log.Error("list unsynced executions", zap.Error(err))
		return false
	}

	now := time.Now().UnixMilli()
	for _, exec := range execs {
		ctx := usageparser.ParseContext{
			LogPath:          exec.LogPath,
			SessionID:        exec.SessionID,
			PiSessionsDir:    s.cfg.PiSessionsDir,
			CodexStoreDir:    s.cfg.CodexStoreDir,
			CodexSessionsDir: s.cfg.CodexSessionsDir,
			StartedAt:        exec.StartedAt,
			CompletedAt:      exec.CompletedAt,
		}
		data, err := usageparser.ParseUsage(ctx)
		if err != nil {
			log.Error("parse usage", zap.String("executionID", exec.ID), zap.Error(err))
		}
		record := &model.UsageRecord{
			ID:                  uuid.New().String(),
			ExecutionID:         exec.ID,
			Model:               data.Model,
			InputTokens:         data.InputTokens,
			OutputTokens:        data.OutputTokens,
			CacheCreationTokens: data.CacheCreationTokens,
			CacheReadTokens:     data.CacheReadTokens,
			TotalTokens:         data.TotalTokens,
			CostUSD:             data.CostUSD,
			SyncedAt:            now,
		}
		if err := s.store.Insert(record); err != nil {
			log.Error("insert usage record", zap.String("executionID", exec.ID), zap.Error(err))
		}
	}

	return len(execs) == s.batchSize
}
