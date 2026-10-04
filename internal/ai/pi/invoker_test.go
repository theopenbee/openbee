package pi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestBuildArgs_NewSession(t *testing.T) {
	args := buildArgs("", "/tmp/session.jsonl", nil)
	want := []string{"--mode", "json", "--session", "/tmp/session.jsonl", "-p", ""}
	assert.Equal(t, want, args)
}

func TestBuildArgs_WithPrompt(t *testing.T) {
	args := buildArgs("hello world", "/tmp/session.jsonl", nil)
	want := []string{"--mode", "json", "--session", "/tmp/session.jsonl", "-p", "hello world"}
	assert.Equal(t, want, args)
}

func TestExtractResultFromLog_BasicResult(t *testing.T) {
	log := `{"type":"turn.started"}
{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"final answer"}]}]}
`
	path := writeTemp(t, log)
	result := ExtractResultFromLog(path)
	assert.Equal(t, "final answer", result)
}

func TestExtractResultFromLog_LastAgentEndWins(t *testing.T) {
	log := `{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"first"}]}]}
{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"last"}]}]}
`
	path := writeTemp(t, log)
	result := ExtractResultFromLog(path)
	assert.Equal(t, "last", result)
}

func TestExtractResultFromLog_SkipsNonTextContent(t *testing.T) {
	log := `{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"x"},{"type":"text","text":"answer"}]}]}
`
	path := writeTemp(t, log)
	result := ExtractResultFromLog(path)
	assert.Equal(t, "answer", result)
}

func TestExtractResultFromLog_Empty(t *testing.T) {
	path := writeTemp(t, "")
	result := ExtractResultFromLog(path)
	assert.Empty(t, result)
}

func TestExtractResultFromLog_NoAgentEnd(t *testing.T) {
	path := writeTemp(t, `{"type":"turn.started"}`+"\n")
	result := ExtractResultFromLog(path)
	assert.Empty(t, result)
}

func TestResolveSessionPath_UsesUUID(t *testing.T) {
	t.Setenv("OPENBEE_URL", "http://localhost:8080")
	sessionID := "4d0ce91b-0856-44e2-b0d7-7765d824bba3"
	inv, err := NewInvoker("true", nil)
	require.NoError(t, err)
	got := inv.sessionFilePath(sessionID)
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".openbee", ".pi", "sessions", sessionID+".jsonl")
	assert.Equal(t, want, got)
}

func TestInvoker_Run_ExitsCleanly(t *testing.T) {
	t.Setenv("OPENBEE_URL", "http://localhost:8080")
	inv, err := NewInvoker("true", nil)
	require.NoError(t, err)
	logPath := filepath.Join(t.TempDir(), "pi.log")

	_, ch, err := inv.Run(context.Background(), t.TempDir(), "hello",
		ai.RunOptions{SessionID: "4d0ce91b-0856-44e2-b0d7-7765d824bba3"}, logPath)
	require.NoError(t, err)

	for range ch {
	}
}

func TestIsStreamingDelta(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"message_update filtered", `{"type":"message_update","message":{}}`, true},
		{"tool_execution_update filtered", `{"type":"tool_execution_update","toolCallId":"x","partialResult":{"content":[]}}`, true},
		{"tool_execution_start not filtered", `{"type":"tool_execution_start","toolCallId":"x","toolName":"bash","args":{}}`, false},
		{"tool_execution_end not filtered", `{"type":"tool_execution_end","toolCallId":"x","result":{"content":[]}}`, false},
		{"message_end not filtered", `{"type":"message_end","message":{"role":"assistant","content":[]}}`, false},
		{"plain JSON not filtered", `{"type":"agent_start"}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isStreamingDelta([]byte(tc.line))
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCheckAgentError_AuthError(t *testing.T) {
	log := `{"type":"session","version":3,"id":"abc","timestamp":"2026-04-15T03:36:31.419Z","cwd":"/tmp"}
{"type":"agent_end","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]},{"role":"assistant","content":[],"stopReason":"error","errorMessage":"401 {\"error\":{\"type\":\"authentication_error\",\"message\":\"The API Key appears to be invalid or may have expired.\"}}"}]}
`
	path := writeTemp(t, log)
	got := checkAgentError(path)
	want := `401 {"error":{"type":"authentication_error","message":"The API Key appears to be invalid or may have expired."}}`
	assert.Equal(t, want, got)
}

func TestCheckAgentError_NoError(t *testing.T) {
	log := `{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"hello"}],"stopReason":"stop"}]}
`
	path := writeTemp(t, log)
	got := checkAgentError(path)
	assert.Empty(t, got)
}

func TestCheckAgentError_EmptyLog(t *testing.T) {
	path := writeTemp(t, "")
	got := checkAgentError(path)
	assert.Empty(t, got)
}

func TestExtractPiError_NonJSONLine(t *testing.T) {
	stderr := `{"type":"session","version":3}
No API key found for unknown.
Use /login or set an API key environment variable.`
	got := extractPiError(stderr, "exit status 1")
	want := "No API key found for unknown."
	assert.Equal(t, want, got)
}

func TestExtractPiError_FallbackOnAllJSON(t *testing.T) {
	stderr := `{"type":"session","version":3}`
	got := extractPiError(stderr, "exit status 1")
	assert.Equal(t, "exit status 1", got)
}

func TestExtractPiError_EmptyStderr(t *testing.T) {
	got := extractPiError("", "exit status 1")
	assert.Equal(t, "exit status 1", got)
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "pi-log-*.jsonl")
	require.NoError(t, err)
	defer f.Close()
	_, err = f.WriteString(content)
	require.NoError(t, err)
	return f.Name()
}
