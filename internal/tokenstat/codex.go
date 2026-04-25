package tokenstat

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
)

type codexParser struct {
	mappingDir string
	codexBase  string
}

func NewCodexParser(mappingDir string) Parser {
	codexBase := os.Getenv("CODEX_HOME")
	if codexBase == "" {
		home, _ := os.UserHomeDir()
		codexBase = filepath.Join(home, ".codex")
	}
	return &codexParser{mappingDir: mappingDir, codexBase: codexBase}
}

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

func (p *codexParser) Parse(sessionID string) ([]SessionTokenUsage, error) {
	data, err := os.ReadFile(filepath.Join(p.mappingDir, sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: read codex mapping for session %s", ErrSessionDataNotFound, sessionID)
		}
		return nil, fmt.Errorf("read codex mapping for session %s: %w", sessionID, err)
	}
	codexSessionID := strings.TrimSpace(string(data))
	if codexSessionID == "" {
		return nil, fmt.Errorf("empty codex session id in mapping for %s", sessionID)
	}
	path, err := findCodexSessionFile(p.codexBase, codexSessionID)
	if err != nil {
		if errors.Is(err, ErrSessionDataNotFound) {
			return nil, fmt.Errorf("%w: codex session file not found for %s", ErrSessionDataNotFound, codexSessionID)
		}
		return nil, err
	}
	return codexParse(sessionID, path)
}

func findCodexSessionFile(codexBase, sessionID string) (string, error) {
	return findWithLegacyFast(
		filepath.Join(codexBase, "sessions"),
		sessionID+".jsonl",
		func(_ string, d os.DirEntry) bool {
			return strings.HasSuffix(d.Name(), ".jsonl") && strings.Contains(d.Name(), sessionID)
		},
	)
}

func codexParse(sessionID, path string) ([]SessionTokenUsage, error) {
	agg := map[string]*SessionTokenUsage{}
	prevByModel := map[string]*codexTokenUsage{}
	currentModel := ""
	err := scanJSONLFile(path, func(data []byte) {
		var line codexJSONLLine
		if err := json.Unmarshal(data, &line); err != nil {
			return
		}
		switch line.Type {
		case "turn_context":
			if line.Payload.Model != "" {
				currentModel = line.Payload.Model
			}
		case "event_msg":
			if line.Payload.Type != "" && line.Payload.Type != "token_count" {
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
			u := getOrCreate(agg, sessionID, ai.EngineCodex, m)
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
	return mapValues(agg), nil
}

func addCodexUsage(dst *SessionTokenUsage, usage codexTokenUsage) {
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
