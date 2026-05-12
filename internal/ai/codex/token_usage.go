package codex

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/utils/sessionfile"
)

// Collector reads the openbee-UUID → codex-thread-ID mapping written by
// SessionStore, then locates the session JSONL under codexBase/sessions/.
type Collector struct {
	mappingDir string
	codexBase  string
}

// defaultCodexBase returns ~/.codex.
func defaultCodexBase() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

// NewCollector builds a Collector using the default mapping directory and
// CODEX_HOME (or ~/.codex if unset).
func NewCollector() *Collector {
	return NewCollectorAt(
		config.DefaultCodexSessionsDir(),
		config.EngineSessionsDir("CODEX_HOME", defaultCodexBase),
	)
}

// NewCollectorAt is a test seam allowing arbitrary mapping/codex roots.
func NewCollectorAt(mappingDir, codexBase string) *Collector {
	return &Collector{mappingDir: mappingDir, codexBase: codexBase}
}

const (
	codexLineTurnContext = "turn_context"
	codexLineEventMsg    = "event_msg"
	codexPayloadTokens   = "token_count"
)

type codexJSONLLine struct {
	Type    string `json:"type"`
	Payload struct {
		Type  string          `json:"type"`
		Model string          `json:"model"`
		Info  *codexTokenInfo `json:"info"`
	} `json:"payload"`
	Info *codexTokenInfo `json:"info"`
}

type codexTokenInfo struct {
	Model     string `json:"model"`
	ModelName string `json:"model_name"`
	Metadata  struct {
		Model string `json:"model"`
	} `json:"metadata"`
	LastTokenUsage  *codexTokenUsage `json:"last_token_usage"`
	TotalTokenUsage *codexTokenUsage `json:"total_token_usage"`
}

type codexTokenUsage struct {
	InputTokens       int64 `json:"input_tokens"`
	OutputTokens      int64 `json:"output_tokens"`
	CachedInputTokens int64 `json:"cached_input_tokens"`
}

func (t *codexTokenUsage) advance(usage codexTokenUsage) {
	t.InputTokens += usage.InputTokens
	t.OutputTokens += usage.OutputTokens
	t.CachedInputTokens += usage.CachedInputTokens
}

func (t *codexTokenUsage) deltaAndSet(total codexTokenUsage) codexTokenUsage {
	delta := codexTokenUsage{
		InputTokens:       total.InputTokens - t.InputTokens,
		OutputTokens:      total.OutputTokens - t.OutputTokens,
		CachedInputTokens: total.CachedInputTokens - t.CachedInputTokens,
	}
	*t = total
	return delta
}

func (l codexJSONLLine) tokenInfo() *codexTokenInfo {
	if l.Payload.Info != nil {
		return l.Payload.Info
	}
	return l.Info
}

func (c *Collector) Collect(_ context.Context, sessionID string) ([]ai.TokenUsage, error) {
	data, err := os.ReadFile(filepath.Join(c.mappingDir, sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: read codex mapping for session %s", ai.ErrSessionDataNotFound, sessionID)
		}
		return nil, fmt.Errorf("read codex mapping for session %s: %w", sessionID, err)
	}
	codexSessionID := strings.TrimSpace(string(data))
	if codexSessionID == "" {
		return nil, fmt.Errorf("empty codex session id in mapping for %s", sessionID)
	}
	path, err := findCodexSessionFile(c.codexBase, codexSessionID)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: codex session file not found for %s", ai.ErrSessionDataNotFound, codexSessionID)
		}
		return nil, err
	}
	return parseCodexFile(path)
}

func findCodexSessionFile(codexBase, sessionID string) (string, error) {
	return sessionfile.FindWithLegacyFast(
		filepath.Join(codexBase, "sessions"),
		sessionID+".jsonl",
		func(_ string, d os.DirEntry) bool {
			return strings.HasSuffix(d.Name(), ".jsonl") && strings.Contains(d.Name(), sessionID)
		},
	)
}

func parseCodexFile(path string) ([]ai.TokenUsage, error) {
	prevByModel := map[string]*codexTokenUsage{}
	currentModel := ""
	usages, err := ai.AggregateUsage[codexJSONLLine](path, func(line codexJSONLLine, agg map[string]*ai.TokenUsage) {
		switch line.Type {
		case codexLineTurnContext:
			if line.Payload.Model != "" {
				currentModel = line.Payload.Model
			}
		case codexLineEventMsg:
			if line.Payload.Type != "" && line.Payload.Type != codexPayloadTokens {
				return
			}
			info := line.tokenInfo()
			if info == nil {
				return
			}
			m := codexResolveModel(info, currentModel)
			if m == "" {
				return
			}
			u := agg[m]
			if u == nil {
				u = &ai.TokenUsage{Model: m}
				agg[m] = u
			}
			if prevByModel[m] == nil {
				prevByModel[m] = &codexTokenUsage{}
			}
			prev := prevByModel[m]
			if info.LastTokenUsage != nil {
				addCodexUsage(u, *info.LastTokenUsage)
				if info.TotalTokenUsage != nil {
					// Codex emits both fields together when a turn is replayed/resumed;
					// the cumulative total is authoritative, so reset prev instead of
					// double-counting by advancing it.
					*prev = *info.TotalTokenUsage
				} else {
					prev.advance(*info.LastTokenUsage)
				}
			} else if info.TotalTokenUsage != nil {
				addCodexUsage(u, prev.deltaAndSet(*info.TotalTokenUsage))
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("scan codex session file: %w", err)
	}
	return usages, nil
}

func addCodexUsage(dst *ai.TokenUsage, usage codexTokenUsage) {
	dst.InputTokens += usage.InputTokens
	dst.OutputTokens += usage.OutputTokens
	dst.CacheReadTokens += usage.CachedInputTokens
}

func codexResolveModel(info *codexTokenInfo, currentModel string) string {
	if info.Model != "" {
		return info.Model
	}
	if info.ModelName != "" {
		return info.ModelName
	}
	if info.Metadata.Model != "" {
		return info.Metadata.Model
	}
	return currentModel
}
