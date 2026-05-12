package pi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
	core "github.com/theopenbee/openbee/internal/ai/core"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/utils/sessionfile"
)

type Collector struct {
	sessionsDir string
}

// NewCollector builds a Collector using PI_AGENT_DIR or the config default.
func NewCollector() *Collector {
	return NewCollectorAt(config.EngineSessionsDir("PI_AGENT_DIR", config.DefaultPiSessionsDir))
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

func (c *Collector) Collect(_ context.Context, sessionID string) ([]ai.TokenUsage, error) {
	path, err := sessionfile.FindWithLegacyFast(c.sessionsDir, sessionID+".jsonl", func(_ string, d os.DirEntry) bool {
		return strings.HasSuffix(d.Name(), "_"+sessionID+".jsonl")
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: pi session file not found for %s", ai.ErrSessionDataNotFound, sessionID)
		}
		return nil, fmt.Errorf("pi session file lookup for %s: %w", sessionID, err)
	}
	return parsePiFile(path)
}

func parsePiFile(path string) ([]ai.TokenUsage, error) {
	usages, err := core.AggregateUsage[piJSONLLine](path, func(line piJSONLLine, agg map[string]*ai.TokenUsage) {
		if line.Type != "message" || line.Message.Role != "assistant" || line.Message.Usage == nil {
			return
		}
		m := line.Message.Model
		u := agg[m]
		if u == nil {
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
	return usages, nil
}
