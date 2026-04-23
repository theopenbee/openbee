package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTempLog(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.log")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func parseCtx(logPath string) ParseContext {
	return ParseContext{LogPath: logPath}
}

// --- Claude ---

func TestParseUsage_Claude_Success(t *testing.T) {
	log := `{"type":"system","subtype":"init","cwd":"/tmp"}
{"type":"assistant","message":{"model":"claude-sonnet-4-6","id":"msg_01","type":"message","role":"assistant","content":[]}}
{"type":"result","subtype":"success","is_error":false,"total_cost_usd":0.35289275,"usage":{"input_tokens":8,"cache_creation_input_tokens":14984,"cache_read_input_tokens":103792,"output_tokens":2849}}`

	data, err := ParseUsage(parseCtx(writeTempLog(t, log)))
	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet-4-6", data.Model)
	assert.Equal(t, int64(8), data.InputTokens)
	assert.Equal(t, int64(2849), data.OutputTokens)
	assert.Equal(t, int64(14984), data.CacheCreationTokens)
	assert.Equal(t, int64(103792), data.CacheReadTokens)
	assert.Equal(t, int64(8+2849+14984+103792), data.TotalTokens)
	assert.InDelta(t, 0.35289275, data.CostUSD, 0.000001)
	assert.Equal(t, "claude", data.Engine)
}

func TestParseUsage_Claude_NoResultEvent(t *testing.T) {
	log := `{"type":"system","subtype":"init","cwd":"/tmp"}
{"type":"assistant","message":{"model":"claude-sonnet-4-6","id":"msg_01","type":"message","role":"assistant","content":[]}}`

	data, err := ParseUsage(parseCtx(writeTempLog(t, log)))
	require.NoError(t, err)
	assert.Equal(t, int64(0), data.TotalTokens)
	assert.Equal(t, float64(0), data.CostUSD)
	assert.Equal(t, "claude", data.Engine)
}

// --- Codex ---

func TestParseUsage_Codex_NoNativeSessions(t *testing.T) {
	log := `{"type":"thread.started","thread_id":"thread_abc123"}
{"type":"item.completed","item":{"type":"agent_message","text":"hello"}}`

	// Empty codex store → returns zero-value gracefully.
	data, err := ParseUsage(parseCtx(writeTempLog(t, log)))
	require.NoError(t, err)
	assert.Equal(t, int64(0), data.TotalTokens)
	assert.Equal(t, "", data.Model)
}

func TestParseUsage_Codex_WithSessionFile(t *testing.T) {
	codexStoreDir := t.TempDir()
	codexSessionsDir := t.TempDir()
	sessionID := "openbee-session-uuid"
	threadID := "thread_codex123"

	// Write thread_id mapping file (openbee's codex session store).
	require.NoError(t, os.WriteFile(filepath.Join(codexStoreDir, sessionID), []byte(threadID), 0o644))

	now := time.Now()
	// Write a codex native session JSONL file containing the thread_id and a token_count event.
	sessionLines := []string{
		`{"type":"thread.started","thread_id":"thread_codex123","timestamp":"` + now.UTC().Format(time.RFC3339Nano) + `"}`,
		`{"type":"turn_context","timestamp":"` + now.UTC().Format(time.RFC3339Nano) + `","payload":{"model":"gpt-5"}}`,
		`{"type":"event_msg","timestamp":"` + now.UTC().Format(time.RFC3339Nano) + `","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1200,"cached_input_tokens":200,"output_tokens":500,"reasoning_output_tokens":0,"total_tokens":1700},"model":"gpt-5"}}}`,
	}
	sessionFile := filepath.Join(codexSessionsDir, "session.jsonl")
	require.NoError(t, os.WriteFile(sessionFile, []byte(strings.Join(sessionLines, "\n")+"\n"), 0o644))

	codexLog := `{"type":"thread.started","thread_id":"thread_codex123"}
{"type":"item.completed","item":{"type":"agent_message","text":"hi"}}`

	ctx := ParseContext{
		LogPath:          writeTempLog(t, codexLog),
		SessionID:        sessionID,
		CodexStoreDir:    codexStoreDir,
		CodexSessionsDir: codexSessionsDir,
		StartedAt:        now.Add(-5 * time.Second).UnixMilli(),
		CompletedAt:      now.Add(5 * time.Second).UnixMilli(),
	}
	data, err := ParseUsage(ctx)
	require.NoError(t, err)
	assert.Equal(t, "gpt-5", data.Model)
	assert.Equal(t, int64(1200), data.InputTokens)
	assert.Equal(t, int64(200), data.CacheReadTokens)
	assert.Equal(t, int64(500), data.OutputTokens)
	assert.Equal(t, int64(1700), data.TotalTokens)
	assert.Equal(t, "codex", data.Engine)
}

// --- Pi ---

func TestParseUsage_Pi_ReadsSessionFile(t *testing.T) {
	piSessionDir := t.TempDir()
	sessionID := "test-session-uuid"

	now := time.Now()
	entry := map[string]any{
		"type":      "message",
		"timestamp": now.UTC().Format(time.RFC3339Nano),
		"message": map[string]any{
			"role":  "assistant",
			"model": "claude-opus-4-5",
			"usage": map[string]any{
				"input":       100,
				"output":      50,
				"cacheRead":   10,
				"cacheWrite":  20,
				"totalTokens": 180,
				"cost":        map[string]any{"total": 0.05},
			},
		},
	}
	line, _ := json.Marshal(entry)
	sessionPath := filepath.Join(piSessionDir, sessionID+".jsonl")
	require.NoError(t, os.WriteFile(sessionPath, append(line, '\n'), 0o644))

	// Pi log (agent_end triggers pi engine detection)
	piLog := `{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"done"}]}]}`

	ctx := ParseContext{
		LogPath:       writeTempLog(t, piLog),
		SessionID:     sessionID,
		PiSessionsDir: piSessionDir,
		StartedAt:     now.Add(-5 * time.Second).UnixMilli(),
		CompletedAt:   now.Add(5 * time.Second).UnixMilli(),
	}
	data, err := ParseUsage(ctx)
	require.NoError(t, err)
	assert.Equal(t, "claude-opus-4-5", data.Model)
	assert.Equal(t, int64(100), data.InputTokens)
	assert.Equal(t, int64(50), data.OutputTokens)
	assert.Equal(t, int64(20), data.CacheCreationTokens)
	assert.Equal(t, int64(10), data.CacheReadTokens)
	assert.Equal(t, int64(180), data.TotalTokens)
	assert.InDelta(t, 0.05, data.CostUSD, 0.0001)
	assert.Equal(t, "pi", data.Engine)
}

func TestParseUsage_Pi_OutsideTimeWindow(t *testing.T) {
	piSessionDir := t.TempDir()
	sessionID := "test-session-uuid-2"

	old := time.Now().Add(-10 * time.Minute)
	entry := map[string]any{
		"type":      "message",
		"timestamp": old.UTC().Format(time.RFC3339Nano),
		"message": map[string]any{
			"role":  "assistant",
			"model": "claude-opus-4-5",
			"usage": map[string]any{
				"input": 100, "output": 50,
			},
		},
	}
	line, _ := json.Marshal(entry)
	sessionPath := filepath.Join(piSessionDir, sessionID+".jsonl")
	require.NoError(t, os.WriteFile(sessionPath, append(line, '\n'), 0o644))

	piLog := `{"type":"agent_end","messages":[]}`
	now := time.Now()
	ctx := ParseContext{
		LogPath:       writeTempLog(t, piLog),
		SessionID:     sessionID,
		PiSessionsDir: piSessionDir,
		StartedAt:     now.Add(-5 * time.Second).UnixMilli(),
		CompletedAt:   now.Add(5 * time.Second).UnixMilli(),
	}
	data, err := ParseUsage(ctx)
	require.NoError(t, err)
	// Message is outside window → zero usage.
	assert.Equal(t, int64(0), data.TotalTokens)
}

func TestParseUsage_Pi_MissingSessionFile(t *testing.T) {
	piLog := `{"type":"agent_end","messages":[]}`
	ctx := ParseContext{
		LogPath:       writeTempLog(t, piLog),
		SessionID:     "nonexistent",
		PiSessionsDir: t.TempDir(),
	}
	data, err := ParseUsage(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), data.TotalTokens)
}

// --- Edge cases ---

func TestParseUsage_EmptyFile(t *testing.T) {
	data, err := ParseUsage(parseCtx(writeTempLog(t, "")))
	require.NoError(t, err)
	assert.Equal(t, int64(0), data.TotalTokens)
}

func TestParseUsage_FileNotFound(t *testing.T) {
	data, err := ParseUsage(parseCtx("/no/such/file.log"))
	require.NoError(t, err)
	assert.Equal(t, int64(0), data.TotalTokens)
}
