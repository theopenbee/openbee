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

type claudeParser struct {
	baseDirs []string
}

func NewClaudeParser() Parser {
	return &claudeParser{baseDirs: claudeBaseDirs()}
}

func claudeBaseDirs() []string {
	if env := os.Getenv("CLAUDE_CONFIG_DIR"); env != "" {
		parts := strings.Split(env, ",")
		dirs := make([]string, 0, len(parts))
		for _, p := range parts {
			dirs = append(dirs, strings.TrimSpace(p))
		}
		return dirs
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

func (p *claudeParser) Parse(sessionID string) ([]SessionTokenUsage, error) {
	for _, base := range p.baseDirs {
		path, err := findClaudeSessionFile(base, sessionID)
		if err == nil {
			return claudeParse(sessionID, path)
		}
		if !errors.Is(err, ErrSessionDataNotFound) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: claude session file not found for %s", ErrSessionDataNotFound, sessionID)
}

func findClaudeSessionFile(base, sessionID string) (string, error) {
	legacyPath := filepath.Join(base, "projects", sessionID+".jsonl")
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath, nil
	}
	return findSessionFile(filepath.Join(base, "projects"), func(_ string, d os.DirEntry) bool {
		return d.Name() == sessionID+".jsonl"
	})
}

func claudeParse(sessionID, path string) ([]SessionTokenUsage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open claude session file: %w", err)
	}
	defer f.Close()

	agg := map[string]*SessionTokenUsage{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 16*1024*1024), 16*1024*1024)

	for scanner.Scan() {
		var line claudeJSONLLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		if line.Message.Model == "" || line.Message.Usage == nil {
			continue
		}
		m := line.Message.Model
		if line.Message.Speed == "fast" {
			m += "-fast"
		}
		u := getOrCreate(agg, sessionID, "claude", m)
		u.InputTokens += line.Message.Usage.InputTokens
		u.OutputTokens += line.Message.Usage.OutputTokens
		u.CacheCreationTokens += line.Message.Usage.CacheCreationInputTokens
		u.CacheReadTokens += line.Message.Usage.CacheReadInputTokens
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan claude session file: %w", err)
	}
	return mapValues(agg), nil
}

func getOrCreate(agg map[string]*SessionTokenUsage, sessionID, agentType, model string) *SessionTokenUsage {
	if u, ok := agg[model]; ok {
		return u
	}
	u := &SessionTokenUsage{SessionID: sessionID, AgentType: agentType, Model: model}
	agg[model] = u
	return u
}

func mapValues(agg map[string]*SessionTokenUsage) []SessionTokenUsage {
	result := make([]SessionTokenUsage, 0, len(agg))
	for _, u := range agg {
		result = append(result, *u)
	}
	return result
}
