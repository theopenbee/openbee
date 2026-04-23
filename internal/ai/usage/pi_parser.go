package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

type piSessionEntry struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		Role  string `json:"role"`
		Model string `json:"model"`
		Usage *struct {
			Input      int64   `json:"input"`
			Output     int64   `json:"output"`
			CacheRead  int64   `json:"cacheRead"`
			CacheWrite int64   `json:"cacheWrite"`
			TotalTokens *int64 `json:"totalTokens"`
			Cost       *struct {
				Total float64 `json:"total"`
			} `json:"cost"`
		} `json:"usage"`
	} `json:"message"`
}

// parsePiUsage reads the pi session JSONL file and sums token usage for assistant
// messages whose timestamp falls within [startedAt, completedAt] (Unix ms).
// Returns zero-value UsageData when the file is missing or no matching entries exist.
func parsePiUsage(sessionFilePath string, startedAt, completedAt int64) (*UsageData, error) {
	f, err := os.Open(sessionFilePath)
	if err != nil {
		return &UsageData{}, nil
	}
	defer f.Close()

	var data UsageData
	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var entry piSessionEntry
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}

		isMessage := entry.Type == "" || entry.Type == "message"
		if !isMessage {
			continue
		}
		if entry.Message.Role != "assistant" || entry.Message.Usage == nil {
			continue
		}

		// Filter by time window when bounds are set.
		if startedAt > 0 && completedAt > 0 && entry.Timestamp != "" {
			t, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
			if err == nil {
				ms := t.UnixMilli()
				if ms < startedAt || ms > completedAt {
					continue
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
	}

	return &data, scanner.Err()
}
