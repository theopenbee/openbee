package usage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTempLog(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.log")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestParseUsageFromLog_Claude_Success(t *testing.T) {
	log := `{"type":"system","subtype":"init","cwd":"/tmp"}
{"type":"assistant","message":{"model":"claude-sonnet-4-6","id":"msg_01","type":"message","role":"assistant","content":[]}}
{"type":"result","subtype":"success","is_error":false,"total_cost_usd":0.35289275,"usage":{"input_tokens":8,"cache_creation_input_tokens":14984,"cache_read_input_tokens":103792,"output_tokens":2849}}`

	data, err := ParseUsageFromLog(writeTempLog(t, log))
	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet-4-6", data.Model)
	assert.Equal(t, int64(8), data.InputTokens)
	assert.Equal(t, int64(2849), data.OutputTokens)
	assert.Equal(t, int64(14984), data.CacheCreationTokens)
	assert.Equal(t, int64(103792), data.CacheReadTokens)
	assert.Equal(t, int64(8+2849+14984+103792), data.TotalTokens)
	assert.InDelta(t, 0.35289275, data.CostUSD, 0.000001)
}

func TestParseUsageFromLog_Claude_NoResultEvent(t *testing.T) {
	// Incomplete log (process killed mid-run) — should return zero-value, no error
	log := `{"type":"system","subtype":"init","cwd":"/tmp"}
{"type":"assistant","message":{"model":"claude-sonnet-4-6","id":"msg_01","type":"message","role":"assistant","content":[]}}`

	data, err := ParseUsageFromLog(writeTempLog(t, log))
	require.NoError(t, err)
	assert.Equal(t, int64(0), data.TotalTokens)
	assert.Equal(t, float64(0), data.CostUSD)
}

func TestParseUsageFromLog_Codex(t *testing.T) {
	log := `{"type":"thread.started","thread_id":"thread_abc123"}
{"type":"item.completed","item":{"type":"agent_message","text":"hello"}}`

	data, err := ParseUsageFromLog(writeTempLog(t, log))
	require.NoError(t, err)
	assert.Equal(t, int64(0), data.TotalTokens)
	assert.Equal(t, "", data.Model)
}

func TestParseUsageFromLog_Pi(t *testing.T) {
	log := `{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"done"}]}]}`

	data, err := ParseUsageFromLog(writeTempLog(t, log))
	require.NoError(t, err)
	assert.Equal(t, int64(0), data.TotalTokens)
}

func TestParseUsageFromLog_EmptyFile(t *testing.T) {
	data, err := ParseUsageFromLog(writeTempLog(t, ""))
	require.NoError(t, err)
	assert.Equal(t, int64(0), data.TotalTokens)
}

func TestParseUsageFromLog_FileNotFound(t *testing.T) {
	data, err := ParseUsageFromLog("/no/such/file.log")
	require.NoError(t, err) // missing file is not an error — returns zero-value
	assert.Equal(t, int64(0), data.TotalTokens)
}
