package tokenstat

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/config"
)

const kimiModel = "kimi"

type kimiParser struct {
	sessionsDir string
}

func NewKimiParser() Parser {
	return &kimiParser{sessionsDir: config.DefaultKimiSessionsDir()}
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

func (p *kimiParser) Parse(sessionID string) ([]SessionTokenUsage, error) {
	matches, err := filepath.Glob(filepath.Join(p.sessionsDir, "*", sessionID, "wire.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("glob kimi session: %w", err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("%w: kimi session file not found for %s", ErrSessionDataNotFound, sessionID)
	}
	return kimiParse(sessionID, matches[0])
}

func kimiParse(sessionID, path string) ([]SessionTokenUsage, error) {
	var last *kimiTokenUsage
	err := scanJSONLFile(path, func(data []byte) {
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
		return nil, fmt.Errorf("%w: no StatusUpdate found in %s", ErrSessionDataNotFound, path)
	}
	return []SessionTokenUsage{{
		SessionID:           sessionID,
		AgentType:           ai.EngineKimi,
		Model:               kimiModel,
		InputTokens:         last.InputOther,
		OutputTokens:        last.Output,
		CacheReadTokens:     last.InputCacheRead,
		CacheCreationTokens: last.InputCacheCreation,
	}}, nil
}
