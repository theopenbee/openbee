package usage

import (
	"encoding/json"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)

type UsageData struct {
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	TotalTokens         int64
	CostUSD             float64
}

// ParseContext carries all information needed to extract token usage for one execution.
type ParseContext struct {
	LogPath          string
	SessionID        string
	PiSessionsDir    string // e.g. ~/.openbee/.pi/sessions
	CodexStoreDir    string // e.g. ~/.openbee/.codex/sessions (openbee uuid→thread_id files)
	CodexSessionsDir string // codex native sessions dir, e.g. ~/.codex/sessions
	StartedAt        int64  // Unix milliseconds
	CompletedAt      int64  // Unix milliseconds
}

type engine int

const (
	engineUnknown engine = iota
	engineClaude
	enginePi
	engineCodex
)

func detectEngine(logPath string) engine {
	f, err := os.Open(logPath)
	if err != nil {
		return engineUnknown
	}
	defer f.Close()

	detected := engineUnknown
	ai.ScanJSONLines(f, func(line string) bool {
		var peek struct {
			Type string `json:"type"`
		}
		if json.Unmarshal([]byte(line), &peek) != nil {
			return true
		}
		switch peek.Type {
		case "assistant", "result", "system":
			detected = engineClaude
			return false
		case "agent_end", "agent_start", "message_end", "turn.started":
			detected = enginePi
			return false
		case "thread.started", "item.completed", "item.created":
			detected = engineCodex
			return false
		}
		return true
	})
	return detected
}

// ParseUsage auto-detects the engine from the log and delegates to the appropriate parser.
// Returns a zero-value UsageData (not an error) when data cannot be determined.
func ParseUsage(ctx ParseContext) (*UsageData, error) {
	eng := detectEngine(ctx.LogPath)
	switch eng {
	case engineClaude:
		return parseClaudeUsage(ctx.LogPath)
	case enginePi:
		sessionFile := filepath.Join(ctx.PiSessionsDir, ctx.SessionID+".jsonl")
		return parsePiUsage(sessionFile, ctx.StartedAt, ctx.CompletedAt)
	case engineCodex:
		return parseCodexUsage(ctx.CodexStoreDir, ctx.CodexSessionsDir, ctx.SessionID, ctx.StartedAt, ctx.CompletedAt)
	default:
		return &UsageData{}, nil
	}
}

func parseClaudeUsage(logPath string) (*UsageData, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return &UsageData{}, nil
	}
	defer f.Close()

	var data UsageData
	var model string

	ai.ScanJSONLines(f, func(line string) bool {
		var peek struct {
			Type string `json:"type"`
		}
		if json.Unmarshal([]byte(line), &peek) != nil {
			return true
		}
		switch peek.Type {
		case "assistant":
			if m := extractClaudeModel(line); m != "" {
				model = m
			}
		case "result":
			extractClaudeResult(line, &data)
			data.Model = model
			return false
		}
		return true
	})

	return &data, nil
}
