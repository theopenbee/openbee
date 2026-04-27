package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/sessionfile"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

const syntheticModel = "<synthetic>"

type Collector struct {
	baseDirs []string
}

// NewCollector builds a Collector using CLAUDE_CONFIG_DIR (colon-separated)
// or the standard ~/.claude and ~/.config/claude locations.
func NewCollector() *Collector {
	return &Collector{baseDirs: claudeBaseDirs()}
}

func claudeBaseDirs() []string {
	if env := os.Getenv("CLAUDE_CONFIG_DIR"); env != "" {
		return utils.SplitAndTrim(env)
	}
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".config", "claude"),
	}
}

type claudeJSONLLine struct {
	Message struct {
		Model string `json:"model"`
		Speed string `json:"speed"`
		Usage *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

func (c *Collector) Collect(_ context.Context, sessionID string) ([]ai.TokenUsage, error) {
	name := sessionID + ".jsonl"
	for _, base := range c.baseDirs {
		path, err := sessionfile.FindWithLegacyFast(
			filepath.Join(base, "projects"),
			name,
			func(_ string, d os.DirEntry) bool { return d.Name() == name },
		)
		if err == nil {
			return parseClaudeFile(path)
		}
		if !errors.Is(err, ai.ErrSessionDataNotFound) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: claude session file not found for %s", ai.ErrSessionDataNotFound, sessionID)
}

func parseClaudeFile(path string) ([]ai.TokenUsage, error) {
	agg := map[string]*ai.TokenUsage{}
	err := sessionfile.ScanJSONLFile(path, func(data []byte) {
		var line claudeJSONLLine
		if err := json.Unmarshal(data, &line); err != nil {
			return
		}
		m := line.Message.Model
		if m == "" || m == syntheticModel || line.Message.Usage == nil {
			return
		}
		if line.Message.Speed == "fast" {
			m += "-fast"
		}
		u, ok := agg[m]
		if !ok {
			u = &ai.TokenUsage{Model: m}
			agg[m] = u
		}
		u.InputTokens += line.Message.Usage.InputTokens
		u.OutputTokens += line.Message.Usage.OutputTokens
		u.CacheCreationTokens += line.Message.Usage.CacheCreationInputTokens
		u.CacheReadTokens += line.Message.Usage.CacheReadInputTokens
	})
	if err != nil {
		return nil, fmt.Errorf("scan claude session file: %w", err)
	}
	return ai.DrainUsageMap(agg), nil
}
