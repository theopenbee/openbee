package usage

import (
	"encoding/json"
	"os"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
)

type piSessionEntry struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		Role  string `json:"role"`
		Model string `json:"model"`
		Usage *struct {
			Input       int64  `json:"input"`
			Output      int64  `json:"output"`
			CacheRead   int64  `json:"cacheRead"`
			CacheWrite  int64  `json:"cacheWrite"`
			TotalTokens *int64 `json:"totalTokens"`
			Cost        *struct {
				Total float64 `json:"total"`
			} `json:"cost"`
		} `json:"usage"`
	} `json:"message"`
}

// Returns zero-value UsageData when the file is missing or no matching entries exist.
func parsePiUsage(sessionFilePath string, startedAt, completedAt int64) (*UsageData, error) {
	f, err := os.Open(sessionFilePath)
	if err != nil {
		return &UsageData{}, nil
	}
	defer f.Close()

	var data UsageData
	ai.ScanJSONLines(f, func(line string) bool {
		var entry piSessionEntry
		if json.Unmarshal([]byte(line), &entry) != nil {
			return true
		}
		if entry.Type != "" && entry.Type != "message" {
			return true
		}
		if entry.Message.Role != "assistant" || entry.Message.Usage == nil {
			return true
		}
		if startedAt > 0 && completedAt > 0 && entry.Timestamp != "" {
			t, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
			if err == nil {
				ms := t.UnixMilli()
				if ms < startedAt || ms > completedAt {
					return true
				}
			}
		}
		u := entry.Message.Usage
		data.InputTokens += u.Input
		data.OutputTokens += u.Output
		data.CacheCreationTokens += u.CacheWrite
		data.CacheReadTokens += u.CacheRead
		if u.TotalTokens != nil {
			data.TotalTokens += *u.TotalTokens
		} else {
			data.TotalTokens += u.Input + u.Output + u.CacheWrite + u.CacheRead
		}
		if u.Cost != nil {
			data.CostUSD += u.Cost.Total
		}
		if entry.Message.Model != "" {
			data.Model = entry.Message.Model
		}
		return true
	})

	return &data, nil
}
