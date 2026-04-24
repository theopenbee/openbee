package tokenstat

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/config"
)

const piModelPrefix = "[pi]"

type piParser struct {
	sessionsDir string
}

func NewPiParser() Parser {
	dir := os.Getenv("PI_AGENT_DIR")
	if dir == "" {
		dir = config.DefaultPiSessionsDir()
	}
	return &piParser{sessionsDir: dir}
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

func (p *piParser) Parse(sessionID string) ([]SessionTokenUsage, error) {
	path := filepath.Join(p.sessionsDir, sessionID+".jsonl")
	usages, err := piParse(sessionID, path)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return usages, err
	}
	found, err := findSessionFile(p.sessionsDir, func(_ string, d os.DirEntry) bool {
		return strings.HasSuffix(d.Name(), "_"+sessionID+".jsonl")
	})
	if err != nil {
		return nil, err
	}
	return piParse(sessionID, found)
}

func piParse(sessionID, path string) ([]SessionTokenUsage, error) {
	agg := map[string]*SessionTokenUsage{}
	err := scanJSONLFile(path, func(data []byte) {
		var line piJSONLLine
		if err := json.Unmarshal(data, &line); err != nil {
			return
		}
		if line.Type != "message" || line.Message.Role != "assistant" || line.Message.Usage == nil {
			return
		}
		m := piModelPrefix + line.Message.Model
		u := getOrCreate(agg, sessionID, ai.EnginePi, m)
		u.InputTokens += line.Message.Usage.Input
		u.OutputTokens += line.Message.Usage.Output
		u.CacheCreationTokens += line.Message.Usage.CacheWrite
		u.CacheReadTokens += line.Message.Usage.CacheRead
	})
	if err != nil {
		return nil, fmt.Errorf("scan pi session file: %w", err)
	}
	return mapValues(agg), nil
}
