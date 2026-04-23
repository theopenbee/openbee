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

type UsageSyncer struct {
	store     usageSyncStore
	interval  time.Duration
	batchSize int
}

func NewUsageSyncer(store usageSyncStore, interval time.Duration, batchSize int) *UsageSyncer {
	return &UsageSyncer{store: store, interval: interval, batchSize: batchSize}
}

func (s *UsageSyncer) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			for s.syncBatch() {
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
		data, err := usageparser.ParseUsageFromLog(exec.LogPath)
		if err != nil {
			log.Error("parse usage log", zap.String("executionID", exec.ID), zap.Error(err))
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
