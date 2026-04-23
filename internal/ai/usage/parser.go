package usage

import (
	"encoding/json"
	"os"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// UsageData holds the parsed token counts and cost from an execution log.
type UsageData struct {
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	TotalTokens         int64
	CostUSD             float64
}

// ParseUsageFromLog reads the log file at logPath, auto-detects the engine
// format, and returns token usage data. Returns a zero-value UsageData (not
// an error) when the file is missing, empty, or contains no token data.
func ParseUsageFromLog(logPath string) (*UsageData, error) {
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
		case "thread.started", "agent_end":
			return false
		}
		return true
	})

	return &data, nil
}
