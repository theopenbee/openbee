package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// codexTokenCount is the payload inside a token_count event_msg.
type codexTokenCount struct {
	InputTokens            int64 `json:"input_tokens"`
	CachedInputTokens      int64 `json:"cached_input_tokens"`
	CacheReadInputTokens   int64 `json:"cache_read_input_tokens"` // alias
	OutputTokens           int64 `json:"output_tokens"`
	ReasoningOutputTokens  int64 `json:"reasoning_output_tokens"`
	TotalTokens            int64 `json:"total_tokens"`
}

type codexEventMsg struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Payload   *struct {
		Type string `json:"type"`
		Info *struct {
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

func subtractCodexUsage(cur, prev *codexTokenCount) *codexTokenCount {
	if prev == nil {
		return cur
	}
	maxz := func(a, b int64) int64 {
		if a-b > 0 {
			return a - b
		}
		return 0
	}
	total := maxz(cur.TotalTokens, prev.TotalTokens)
	if total == 0 {
		total = maxz(cur.InputTokens, prev.InputTokens) + maxz(cur.OutputTokens, prev.OutputTokens)
	}
	return &codexTokenCount{
		InputTokens:           maxz(cur.InputTokens, prev.InputTokens),
		CachedInputTokens:     maxz(cur.CachedInputTokens, prev.CachedInputTokens),
		OutputTokens:          maxz(cur.OutputTokens, prev.OutputTokens),
		ReasoningOutputTokens: maxz(cur.ReasoningOutputTokens, prev.ReasoningOutputTokens),
		TotalTokens:           total,
	}
}

// parseCodexSessionFile parses a single codex native session JSONL file and
// accumulates token usage for events within [startedAt, completedAt] (Unix ms).
func parseCodexSessionFile(path string, startedAt, completedAt int64) (*UsageData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var data UsageData
	var prevTotals *codexTokenCount
	var currentModel string

	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, 2*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var ev codexEventMsg
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}

		// Track model from turn_context entries.
		if ev.Type == "turn_context" {
			var ctx struct {
				Payload *struct {
					Model     string `json:"model"`
					ModelName string `json:"model_name"`
				} `json:"payload"`
			}
			if json.Unmarshal([]byte(line), &ctx) == nil && ctx.Payload != nil {
				if ctx.Payload.Model != "" {
					currentModel = ctx.Payload.Model
				} else if ctx.Payload.ModelName != "" {
					currentModel = ctx.Payload.ModelName
				}
			}
			continue
		}

		if ev.Type != "event_msg" || ev.Payload == nil || ev.Payload.Type != "token_count" {
			continue
		}

		// Time-window filter.
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

		// Prefer last_token_usage (per-event delta); fall back to total minus previous.
		var delta *codexTokenCount
		if info.LastTokenUsage != nil {
			delta = normalizeCodexUsage(info.LastTokenUsage)
		} else if info.TotalTokenUsage != nil {
			total := normalizeCodexUsage(info.TotalTokenUsage)
			delta = subtractCodexUsage(total, prevTotals)
		}
		if info.TotalTokenUsage != nil {
			prevTotals = normalizeCodexUsage(info.TotalTokenUsage)
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

		// Resolve model: prefer info fields, then turn_context.
		if info.Model != "" {
			data.Model = info.Model
		} else if info.ModelName != "" {
			data.Model = info.ModelName
		} else if currentModel != "" {
			data.Model = currentModel
		}
	}

	return &data, scanner.Err()
}

// parseCodexUsage looks up the codex thread_id for the given openbee session,
// searches the codex native sessions directory for a matching session file, then
// parses token usage events within the execution time window.
func parseCodexUsage(codexStoreDir, codexSessionsDir, sessionID string, startedAt, completedAt int64) (*UsageData, error) {
	// Step 1: Read codex thread_id from openbee's codex session store.
	threadIDBytes, err := os.ReadFile(filepath.Join(codexStoreDir, sessionID))
	if err != nil {
		return &UsageData{}, nil
	}
	threadID := strings.TrimSpace(string(threadIDBytes))
	if threadID == "" {
		return &UsageData{}, nil
	}

	// Step 2: Glob the codex native sessions directory for JSONL files.
	// Also try one level deep since codex may organise sessions into subdirectories.
	matches, err := filepath.Glob(filepath.Join(codexSessionsDir, "*.jsonl"))
	if err != nil {
		return &UsageData{}, nil
	}
	subMatches, _ := filepath.Glob(filepath.Join(codexSessionsDir, "*", "*.jsonl"))
	matches = append(matches, subMatches...)

	// Step 3: Find the session file that contains the thread_id, then parse it.
	var combined UsageData
	found := false
	for _, sessionFile := range matches {
		if !fileContainsThreadID(sessionFile, threadID) {
			continue
		}
		d, err := parseCodexSessionFile(sessionFile, startedAt, completedAt)
		if err != nil {
			continue
		}
		combined.InputTokens += d.InputTokens
		combined.CacheReadTokens += d.CacheReadTokens
		combined.CacheCreationTokens += d.CacheCreationTokens
		combined.OutputTokens += d.OutputTokens
		combined.TotalTokens += d.TotalTokens
		combined.CostUSD += d.CostUSD
		if d.Model != "" {
			combined.Model = d.Model
		}
		found = true
	}
	if !found {
		return &UsageData{}, nil
	}
	return &combined, nil
}

// fileContainsThreadID does a quick scan of the file looking for the thread_id string.
func fileContainsThreadID(path, threadID string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	needle := `"` + threadID + `"`
	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, 512*1024)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), needle) {
			return true
		}
	}
	return false
}
