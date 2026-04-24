package tokenstat

import (
	"bufio"
	"encoding/json"
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
		Model string `json:"model"`
	} `json:"payload"`
	Info struct {
		Model     string `json:"model"`
		ModelName string `json:"model_name"`
		Metadata  struct {
			Model string `json:"model"`
		} `json:"metadata"`
		LastTokenUsage *struct {
			InputTokens       int64 `json:"input_tokens"`
			OutputTokens      int64 `json:"output_tokens"`
			CachedInputTokens int64 `json:"cached_input_tokens"`
		} `json:"last_token_usage"`
		TotalTokenUsage *struct {
			InputTokens       int64 `json:"input_tokens"`
			OutputTokens      int64 `json:"output_tokens"`
			CachedInputTokens int64 `json:"cached_input_tokens"`
		} `json:"total_token_usage"`
	} `json:"info"`
}

func (p *codexParser) Parse(sessionID string) ([]SessionTokenUsage, error) {
	data, err := os.ReadFile(filepath.Join(p.mappingDir, sessionID))
	if err != nil {
		return nil, fmt.Errorf("read codex mapping for session %s: %w", sessionID, err)
	}
	codexSessionID := strings.TrimSpace(string(data))
	if codexSessionID == "" {
		return nil, fmt.Errorf("empty codex session id in mapping for %s", sessionID)
	}
	path := filepath.Join(p.codexBase, "sessions", codexSessionID+".jsonl")
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("codex session file not found: %s", path)
	}
	return codexParse(sessionID, path)
}

func codexParse(sessionID, path string) ([]SessionTokenUsage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open codex session file: %w", err)
	}
	defer f.Close()

	agg := map[string]*SessionTokenUsage{}
	currentModel := ""
	var prevInput, prevOutput, prevCached int64

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

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
			m := codexResolveModel(line, currentModel)
			if m == "" {
				continue
			}
			u := getOrCreate(agg, sessionID, "codex", m)
			if line.Info.LastTokenUsage != nil {
				ltu := line.Info.LastTokenUsage
				u.InputTokens += ltu.InputTokens
				u.OutputTokens += ltu.OutputTokens
				u.CacheReadTokens += ltu.CachedInputTokens
			} else if line.Info.TotalTokenUsage != nil {
				ttu := line.Info.TotalTokenUsage
				u.InputTokens += ttu.InputTokens - prevInput
				u.OutputTokens += ttu.OutputTokens - prevOutput
				u.CacheReadTokens += ttu.CachedInputTokens - prevCached
				prevInput = ttu.InputTokens
				prevOutput = ttu.OutputTokens
				prevCached = ttu.CachedInputTokens
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan codex session file: %w", err)
	}
	return mapValues(agg), nil
}

func codexResolveModel(line codexJSONLLine, currentModel string) string {
	if line.Info.Model != "" {
		return line.Info.Model
	}
	if line.Info.ModelName != "" {
		return line.Info.ModelName
	}
	if line.Info.Metadata.Model != "" {
		return line.Info.Metadata.Model
	}
	return currentModel
}
