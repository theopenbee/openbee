package pi

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestBuildArgs_NewSession(t *testing.T) {
	args := buildArgs("", "/tmp/session.jsonl", "", nil)
	want := []string{"--mode", "json", "--session", "/tmp/session.jsonl", "-p", ""}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildArgs_WithPrompt(t *testing.T) {
	args := buildArgs("hello world", "/tmp/session.jsonl", "", nil)
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
	result := ExtractResultFromLog(path)
	if result != "final answer" {
		t.Errorf("got %q, want %q", result, "final answer")
	}
}

func TestExtractResultFromLog_LastAgentEndWins(t *testing.T) {
	log := `{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"first"}]}]}
{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"last"}]}]}
`
	path := writeTemp(t, log)
	result := ExtractResultFromLog(path)
	if result != "last" {
		t.Errorf("got %q, want %q", result, "last")
	}
}

func TestExtractResultFromLog_SkipsNonTextContent(t *testing.T) {
	log := `{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"x"},{"type":"text","text":"answer"}]}]}
`
	path := writeTemp(t, log)
	result := ExtractResultFromLog(path)
	if result != "answer" {
		t.Errorf("got %q, want %q", result, "answer")
	}
}

func TestExtractResultFromLog_Empty(t *testing.T) {
	path := writeTemp(t, "")
	result := ExtractResultFromLog(path)
	if result != "" {
		t.Errorf("got %q, want %q", result, "")
	}
}

func TestExtractResultFromLog_NoAgentEnd(t *testing.T) {
	path := writeTemp(t, `{"type":"turn.started"}`+"\n")
	result := ExtractResultFromLog(path)
	if result != "" {
		t.Errorf("got %q, want %q", result, "")
	}
}

func TestResolveSessionPath_UsesUUID(t *testing.T) {
	t.Setenv("OPENBEE_URL", "http://localhost:8080")
	sessionID := "4d0ce91b-0856-44e2-b0d7-7765d824bba3"
	inv, err := NewInvoker("true", nil)
	if err != nil {
		t.Fatalf("NewInvoker: %v", err)
	}
	got := inv.sessionFilePath(sessionID)
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".openbee", ".pi", "sessions", sessionID+".jsonl")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInvoker_Run_ExitsCleanly(t *testing.T) {
	t.Setenv("OPENBEE_URL", "http://localhost:8080")
	inv, err := NewInvoker("true", nil)
	if err != nil {
		t.Fatalf("NewInvoker: %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "pi.log")

	_, ch, err := inv.Run(context.Background(), t.TempDir(), "hello",
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

func TestBuildArgs_OmitsSystemPromptWhenEmpty(t *testing.T) {
	args := buildArgs("hi", "/tmp/sess.jsonl", "", nil)
	for _, a := range args {
		if a == "--append-system-prompt" {
			t.Errorf("unexpected --append-system-prompt in args: %v", args)
		}
	}
}

func TestBuildArgs_AppendsSystemPromptFlag(t *testing.T) {
	args := buildArgs("hi", "/tmp/sess.jsonl", "be terse", nil)
	idx := slices.Index(args, "--append-system-prompt")
	if idx < 0 || idx == len(args)-1 {
		t.Fatalf("expected --append-system-prompt with value, got %v", args)
	}
	if args[idx+1] != "be terse" {
		t.Errorf("expected value %q, got %q", "be terse", args[idx+1])
	}
}

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
