package pi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/sessionfile"
	"github.com/theopenbee/openbee/internal/infra/config"
)

// Collector extracts token usage from pi JSONL session files.
type Collector struct {
	sessionsDir string
}

// NewCollector builds a Collector using PI_AGENT_DIR or the config default.
func NewCollector() *Collector {
	dir := os.Getenv("PI_AGENT_DIR")
	if dir == "" {
		dir = config.DefaultPiSessionsDir()
	}
	return NewCollectorAt(dir)
}

// NewCollectorAt is a test seam allowing an arbitrary sessions root.
func NewCollectorAt(dir string) *Collector {
	return &Collector{sessionsDir: dir}
}

type piJSONLLine struct {
	Type    string `json:"type"`
	Message struct {
		Role  string `json:"role"`
		Model string `json:"model"`
		Usage *struct {
			Input      int64 `json:"input"`
			Output     int64 `json:"output"`
			CacheWrite int64 `json:"cacheWrite"`
			CacheRead  int64 `json:"cacheRead"`
		} `json:"usage"`
	} `json:"message"`
}

// Collect implements the per-engine collection contract.
func (c *Collector) Collect(_ context.Context, sessionID string) ([]ai.TokenUsage, error) {
	path, err := sessionfile.FindWithLegacyFast(c.sessionsDir, sessionID+".jsonl", func(_ string, d os.DirEntry) bool {
		return strings.HasSuffix(d.Name(), "_"+sessionID+".jsonl")
	})
	if err != nil {
		if errors.Is(err, ai.ErrSessionDataNotFound) {
			return nil, fmt.Errorf("%w: pi session file not found for %s", ai.ErrSessionDataNotFound, sessionID)
		}
		return nil, fmt.Errorf("pi session file lookup for %s: %w", sessionID, err)
	}
	return parsePiFile(path)
}

func parsePiFile(path string) ([]ai.TokenUsage, error) {
	agg := map[string]*ai.TokenUsage{}
	err := sessionfile.ScanJSONLFile(path, func(data []byte) {
		var line piJSONLLine
		if err := json.Unmarshal(data, &line); err != nil {
			return
		}
		if line.Type != "message" || line.Message.Role != "assistant" || line.Message.Usage == nil {
			return
		}
		m := line.Message.Model
		u, ok := agg[m]
		if !ok {
			u = &ai.TokenUsage{Model: m}
			agg[m] = u
		}
		u.InputTokens += line.Message.Usage.Input
		u.OutputTokens += line.Message.Usage.Output
		u.CacheCreationTokens += line.Message.Usage.CacheWrite
		u.CacheReadTokens += line.Message.Usage.CacheRead
	})
	if err != nil {
		return nil, fmt.Errorf("scan pi session file: %w", err)
	}
	out := make([]ai.TokenUsage, 0, len(agg))
	for _, u := range agg {
		out = append(out, *u)
	}
	return out, nil
}
