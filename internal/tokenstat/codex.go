package tokenstat

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

type codexTotals struct {
	input  int64
	output int64
	cached int64
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
	legacyPath := filepath.Join(codexBase, "sessions", sessionID+".jsonl")
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath, nil
	}
	return findSessionFile(filepath.Join(codexBase, "sessions"), func(_ string, d os.DirEntry) bool {
		return strings.HasSuffix(d.Name(), ".jsonl") && strings.Contains(d.Name(), sessionID)
	})
}

func codexParse(sessionID, path string) ([]SessionTokenUsage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open codex session file: %w", err)
	}
	defer f.Close()

	agg := map[string]*SessionTokenUsage{}
	currentModel := ""
	var prev codexTotals

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 16*1024*1024), 16*1024*1024)

	for scanner.Scan() {
		var line codexJSONLLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		switch line.Type {
		case "turn_context":
			if line.Payload.Model != "" {
				currentModel = line.Payload.Model
			}
		case "event_msg":
			if line.Payload.Type != "" && line.Payload.Type != "token_count" {
				continue
			}
			info := line.tokenInfo()
			if info == nil {
				continue
			}
			m := codexResolveModel(info, currentModel)
			if m == "" {
				continue
			}
			u := getOrCreate(agg, sessionID, "codex", m)
			if info.LastTokenUsage != nil {
				addCodexUsage(u, *info.LastTokenUsage)
				prev.advance(info.LastTokenUsage)
				if info.TotalTokenUsage != nil {
					prev.set(info.TotalTokenUsage)
				}
			} else if info.TotalTokenUsage != nil {
				addCodexUsage(u, prev.deltaAndSet(info.TotalTokenUsage))
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan codex session file: %w", err)
	}
	return mapValues(agg), nil
}

func addCodexUsage(dst *SessionTokenUsage, usage codexTokenUsage) {
	dst.InputTokens += usage.InputTokens
	dst.OutputTokens += usage.OutputTokens
	dst.CacheReadTokens += usage.CachedInputTokens
}

func (t *codexTotals) advance(usage *codexTokenUsage) {
	t.input += usage.InputTokens
	t.output += usage.OutputTokens
	t.cached += usage.CachedInputTokens
}

func (t *codexTotals) set(usage *codexTokenUsage) {
	t.input = usage.InputTokens
	t.output = usage.OutputTokens
	t.cached = usage.CachedInputTokens
}

func (t *codexTotals) deltaAndSet(total *codexTokenUsage) codexTokenUsage {
	delta := codexTokenUsage{
		InputTokens:       total.InputTokens - t.input,
		OutputTokens:      total.OutputTokens - t.output,
		CachedInputTokens: total.CachedInputTokens - t.cached,
	}
	t.set(total)
	return delta
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
