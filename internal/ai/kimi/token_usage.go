package kimi

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/sessionfile"
	"github.com/theopenbee/openbee/internal/infra/config"
)

const kimiModel = "kimi"

// Collector extracts token usage from kimi wire.jsonl files.
type Collector struct {
	sessionsDir string
}

// NewCollector builds a Collector at the default sessions root.
func NewCollector() *Collector {
	return NewCollectorAt(config.DefaultKimiSessionsDir())
}

// NewCollectorAt is a test seam allowing arbitrary roots.
func NewCollectorAt(dir string) *Collector {
	return &Collector{sessionsDir: dir}
}

type kimiTokenUsage struct {
	InputOther         int64 `json:"input_other"`
	Output             int64 `json:"output"`
	InputCacheRead     int64 `json:"input_cache_read"`
	InputCacheCreation int64 `json:"input_cache_creation"`
}

type kimiJSONLLine struct {
	Message struct {
		Type    string `json:"type"`
		Payload struct {
			TokenUsage *kimiTokenUsage `json:"token_usage"`
		} `json:"payload"`
	} `json:"message"`
}

// Collect implements the per-engine collection contract.
func (c *Collector) Collect(_ context.Context, sessionID string) ([]ai.TokenUsage, error) {
	matches, err := filepath.Glob(filepath.Join(c.sessionsDir, "*", sessionID, "wire.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("glob kimi session: %w", err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("%w: kimi session file not found for %s", ai.ErrSessionDataNotFound, sessionID)
	}
	return parseKimiFile(matches[0])
}

func parseKimiFile(path string) ([]ai.TokenUsage, error) {
	var last *kimiTokenUsage
	err := sessionfile.ScanJSONLFile(path, func(data []byte) {
		var line kimiJSONLLine
		if err := json.Unmarshal(data, &line); err != nil {
			return
		}
		if line.Message.Type != "StatusUpdate" || line.Message.Payload.TokenUsage == nil {
			return
		}
		last = line.Message.Payload.TokenUsage
	})
	if err != nil {
		return nil, fmt.Errorf("scan kimi session file: %w", err)
	}
	if last == nil {
		return nil, fmt.Errorf("%w: no StatusUpdate found in %s", ai.ErrSessionDataNotFound, path)
	}
	return []ai.TokenUsage{{
		Model:               kimiModel,
		InputTokens:         last.InputOther,
		OutputTokens:        last.Output,
		CacheReadTokens:     last.InputCacheRead,
		CacheCreationTokens: last.InputCacheCreation,
	}}, nil
}
