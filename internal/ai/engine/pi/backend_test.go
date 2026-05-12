package pi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "pi-log-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func writePiTempFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestBuildArgs_NewSession(t *testing.T) {
	args := buildArgs("", "/tmp/session.jsonl", nil)
	want := []string{"--mode", "json", "--session", "/tmp/session.jsonl", "-p", ""}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildArgs_WithPrompt(t *testing.T) {
	args := buildArgs("hello world", "/tmp/session.jsonl", nil)
	want := []string{"--mode", "json", "--session", "/tmp/session.jsonl", "-p", "hello world"}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestExtractResultFromLog_BasicResult(t *testing.T) {
	log := `{"type":"turn.started"}
{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"final answer"}]}]}
`
	path := writeTemp(t, log)
	result := (&Backend{}).Extract(path)
	if result != "final answer" {
		t.Errorf("got %q, want %q", result, "final answer")
	}
}

func TestExtractResultFromLog_LastAgentEndWins(t *testing.T) {
	log := `{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"first"}]}]}
{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"last"}]}]}
`
	path := writeTemp(t, log)
	result := (&Backend{}).Extract(path)
	if result != "last" {
		t.Errorf("got %q, want %q", result, "last")
	}
}

func TestExtractResultFromLog_SkipsNonTextContent(t *testing.T) {
	log := `{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"x"},{"type":"text","text":"answer"}]}]}
`
	path := writeTemp(t, log)
	result := (&Backend{}).Extract(path)
	if result != "answer" {
		t.Errorf("got %q, want %q", result, "answer")
	}
}

func TestExtractResultFromLog_Empty(t *testing.T) {
	path := writeTemp(t, "")
	result := (&Backend{}).Extract(path)
	if result != "" {
		t.Errorf("got %q, want %q", result, "")
	}
}

func TestExtractResultFromLog_NoAgentEnd(t *testing.T) {
	path := writeTemp(t, `{"type":"turn.started"}`+"\n")
	result := (&Backend{}).Extract(path)
	if result != "" {
		t.Errorf("got %q, want %q", result, "")
	}
}

func TestResolveSessionPath_UsesUUID(t *testing.T) {
	t.Setenv("OPENBEE_URL", "http://localhost:8080")
	sessionID := "4d0ce91b-0856-44e2-b0d7-7765d824bba3"
	b, err := NewBackend("true", nil)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	got := b.sessionFilePath(sessionID)
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".openbee", ".pi", "sessions", sessionID+".jsonl")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInvoker_Run_ExitsCleanly(t *testing.T) {
	t.Setenv("OPENBEE_URL", "http://localhost:8080")
	b, err := NewBackend("true", nil)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "pi.log")

	_, ch, err := b.Run(context.Background(), t.TempDir(), "hello",
		ai.RunOptions{SessionID: "4d0ce91b-0856-44e2-b0d7-7765d824bba3"}, logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

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
			if got := isStreamingDelta([]byte(tc.line)); got != tc.want {
				t.Errorf("isStreamingDelta(%q) = %v, want %v", tc.line, got, tc.want)
			}
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
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCheckAgentError_NoError(t *testing.T) {
	log := `{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"hello"}],"stopReason":"stop"}]}
`
	path := writeTemp(t, log)
	got := checkAgentError(path)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestCheckAgentError_EmptyLog(t *testing.T) {
	path := writeTemp(t, "")
	got := checkAgentError(path)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractPiError_NonJSONLine(t *testing.T) {
	stderr := `{"type":"session","version":3}
No API key found for unknown.
Use /login or set an API key environment variable.`
	got := extractPiError(stderr, "exit status 1")
	want := "No API key found for unknown."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractPiError_FallbackOnAllJSON(t *testing.T) {
	stderr := `{"type":"session","version":3}`
	got := extractPiError(stderr, "exit status 1")
	if got != "exit status 1" {
		t.Errorf("got %q, want %q", got, "exit status 1")
	}
}

func TestExtractPiError_EmptyStderr(t *testing.T) {
	got := extractPiError("", "exit status 1")
	if got != "exit status 1" {
		t.Errorf("got %q, want %q", got, "exit status 1")
	}
}

func TestPiCollector_Collect_AggregatesByModel(t *testing.T) {
	sessionsDir := t.TempDir()
	sessionID := "pi-sess-abc123"

	writePiTempFile(t, sessionsDir, "20250101_"+sessionID+".jsonl", `{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":100,"output":50,"cacheWrite":10,"cacheRead":5}}}
{"type":"message","message":{"role":"user","content":"hello"}}
{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":200,"output":80,"cacheWrite":0,"cacheRead":15}}}
{"type":"message","message":{"role":"assistant","model":"claude-3-opus","usage":{"input":300,"output":100,"cacheWrite":5,"cacheRead":0}}}
{"type":"other","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":999,"output":999}}}
`)
	t.Setenv("PI_AGENT_DIR", sessionsDir)
	collector := NewBackendAt("", nil, sessionsDir)

	usages, err := collector.Collect(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	byModel := map[string]ai.TokenUsage{}
	for _, u := range usages {
		byModel[u.Model] = u
	}

	sonnet := byModel["claude-3-5-sonnet"]
	if sonnet.InputTokens != 300 {
		t.Errorf("sonnet InputTokens: want 300, got %d", sonnet.InputTokens)
	}
	if sonnet.OutputTokens != 130 {
		t.Errorf("sonnet OutputTokens: want 130, got %d", sonnet.OutputTokens)
	}
	if sonnet.CacheCreationTokens != 10 {
		t.Errorf("sonnet CacheCreationTokens: want 10, got %d", sonnet.CacheCreationTokens)
	}
	if sonnet.CacheReadTokens != 20 {
		t.Errorf("sonnet CacheReadTokens: want 20, got %d", sonnet.CacheReadTokens)
	}

	opus := byModel["claude-3-opus"]
	if opus.InputTokens != 300 {
		t.Errorf("opus InputTokens: want 300, got %d", opus.InputTokens)
	}
}

func TestPiCollector_Collect_SkipsNonAssistantAndWrongType(t *testing.T) {
	sessionsDir := t.TempDir()
	sessionID := "skip-test"

	writePiTempFile(t, sessionsDir, sessionID+".jsonl", `{"type":"message","message":{"role":"user","model":"claude-3-5-sonnet","usage":{"input":999,"output":999}}}
{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":100,"output":50}}}
{"type":"other","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":999,"output":999}}}
`)
	t.Setenv("PI_AGENT_DIR", sessionsDir)

	usages, err := NewBackendAt("", nil, sessionsDir).Collect(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage (only assistant+message), got %d", len(usages))
	}
	if usages[0].InputTokens != 100 {
		t.Errorf("InputTokens: want 100, got %d", usages[0].InputTokens)
	}
}

func TestPiCollector_Collect_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := NewBackendAt("", nil, dir).Collect(context.Background(), "nonexistent-session")
	if !errors.Is(err, ai.ErrSessionDataNotFound) {
		t.Fatalf("expected ErrSessionDataNotFound, got %v", err)
	}
}

func TestPiCollector_Collect_DirectoryNotFound(t *testing.T) {
	_, err := NewBackendAt("", nil, t.TempDir()+"/missing").Collect(context.Background(), "nonexistent-session")
	if !errors.Is(err, ai.ErrSessionDataNotFound) {
		t.Fatalf("expected ErrSessionDataNotFound, got %v", err)
	}
}

func TestPiCollector_Collect_UsesOpenbeeDefaultSessionsDir(t *testing.T) {
	home := t.TempDir()
	sessionID := "default-dir-session"
	t.Setenv("HOME", home)
	t.Setenv("PI_AGENT_DIR", "")

	writePiTempFile(t, home, ".openbee/.pi/sessions/"+sessionID+".jsonl", `{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":100,"output":50}}}`)

	sessionsDir := home + "/.openbee/.pi/sessions"
	usages, err := NewBackendAt("", nil, sessionsDir).Collect(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].InputTokens != 100 || usages[0].OutputTokens != 50 {
		t.Fatalf("unexpected usage: %+v", usages[0])
	}
}
