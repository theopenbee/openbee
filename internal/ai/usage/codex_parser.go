package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type codexTokenCount struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	CacheReadInputTokens  int64 `json:"cache_read_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
	TotalTokens           int64 `json:"total_tokens"`
}

type codexEventMsg struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Payload   *struct {
		Type      string `json:"type"`
		Model     string `json:"model"`      // populated on turn_context events
		ModelName string `json:"model_name"` // populated on turn_context events
		Info      *struct {
			LastTokenUsage  *codexTokenCount `json:"last_token_usage"`
			TotalTokenUsage *codexTokenCount `json:"total_token_usage"`
			Model           string           `json:"model"`
			ModelName       string           `json:"model_name"`
		} `json:"info"`
	} `json:"payload"`
}

func normalizeCodexUsage(tc *codexTokenCount) *codexTokenCount {
	if tc == nil {
		return nil
	}
	cached := tc.CachedInputTokens
	if cached == 0 {
		cached = tc.CacheReadInputTokens
	}
	total := tc.TotalTokens
	if total == 0 {
		total = tc.InputTokens + tc.OutputTokens
	}
	return &codexTokenCount{
		InputTokens:           tc.InputTokens,
		CachedInputTokens:     cached,
		OutputTokens:          tc.OutputTokens,
		ReasoningOutputTokens: tc.ReasoningOutputTokens,
		TotalTokens:           total,
	}
}

func clampedSub(a, b int64) int64 {
	if a-b > 0 {
		return a - b
	}
	return 0
}

func coalesceModel(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func subtractCodexUsage(cur, prev *codexTokenCount) *codexTokenCount {
	if prev == nil {
		return cur
	}
	total := clampedSub(cur.TotalTokens, prev.TotalTokens)
	if total == 0 {
		total = clampedSub(cur.InputTokens, prev.InputTokens) + clampedSub(cur.OutputTokens, prev.OutputTokens)
	}
	return &codexTokenCount{
		InputTokens:           clampedSub(cur.InputTokens, prev.InputTokens),
		CachedInputTokens:     clampedSub(cur.CachedInputTokens, prev.CachedInputTokens),
		OutputTokens:          clampedSub(cur.OutputTokens, prev.OutputTokens),
		ReasoningOutputTokens: clampedSub(cur.ReasoningOutputTokens, prev.ReasoningOutputTokens),
		TotalTokens:           total,
	}
}

// threadID is verified inline to avoid a separate pre-scan pass.
func parseCodexSessionFile(path, threadID string, startedAt, completedAt int64) (*UsageData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var data UsageData
	var prevTotals *codexTokenCount
	var currentModel string
	var foundThreadID bool
	needle := `"` + threadID + `"`

	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, 2*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		if !foundThreadID && strings.Contains(line, needle) {
			foundThreadID = true
		}

		var ev codexEventMsg
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}

		if ev.Type == "turn_context" && ev.Payload != nil {
			currentModel = coalesceModel(ev.Payload.Model, ev.Payload.ModelName)
			continue
		}

		if ev.Type != "event_msg" || ev.Payload == nil || ev.Payload.Type != "token_count" {
			continue
		}

		if startedAt > 0 && completedAt > 0 && ev.Timestamp != "" {
			t, err := time.Parse(time.RFC3339Nano, ev.Timestamp)
			if err == nil {
				ms := t.UnixMilli()
				if ms < startedAt || ms > completedAt {
					// Still update prevTotals so deltas stay correct.
					if ev.Payload.Info != nil {
						prevTotals = normalizeCodexUsage(ev.Payload.Info.TotalTokenUsage)
					}
					continue
				}
			}
		}

		info := ev.Payload.Info
		if info == nil {
			continue
		}

		var delta *codexTokenCount
		if info.LastTokenUsage != nil {
			delta = normalizeCodexUsage(info.LastTokenUsage)
		}
		if info.TotalTokenUsage != nil {
			normalized := normalizeCodexUsage(info.TotalTokenUsage)
			if delta == nil {
				delta = subtractCodexUsage(normalized, prevTotals)
			}
			prevTotals = normalized
		}

		if delta == nil {
			continue
		}
		if delta.InputTokens == 0 && delta.OutputTokens == 0 {
			continue
		}

		data.InputTokens += delta.InputTokens
		data.CacheReadTokens += delta.CachedInputTokens
		data.OutputTokens += delta.OutputTokens
		data.TotalTokens += delta.TotalTokens

		if m := coalesceModel(coalesceModel(info.Model, info.ModelName), currentModel); m != "" {
			data.Model = m
		}
	}

	if !foundThreadID {
		return nil, nil
	}
	return &data, scanner.Err()
}

func parseCodexUsage(codexStoreDir, codexSessionsDir, sessionID string, startedAt, completedAt int64) (*UsageData, error) {
	threadIDBytes, err := os.ReadFile(filepath.Join(codexStoreDir, sessionID))
	if err != nil {
		return &UsageData{}, nil
	}
	threadID := strings.TrimSpace(string(threadIDBytes))
	if threadID == "" {
		return &UsageData{}, nil
	}

	matches, err := filepath.Glob(filepath.Join(codexSessionsDir, "*.jsonl"))
	if err != nil {
		return &UsageData{}, nil
	}
	// Also try one level deep since codex may organise sessions into subdirectories.
	subMatches, _ := filepath.Glob(filepath.Join(codexSessionsDir, "*", "*.jsonl"))
	matches = append(matches, subMatches...)

	for _, sessionFile := range matches {
		d, err := parseCodexSessionFile(sessionFile, threadID, startedAt, completedAt)
		if err != nil || d == nil {
			continue
		}
		return d, nil
	}
	return &UsageData{}, nil
}
