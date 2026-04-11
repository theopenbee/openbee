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
	args := buildArgs("", "/tmp/session.jsonl")
	want := []string{"--mode", "json", "--session", "/tmp/session.jsonl", "-p", ""}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildArgs_WithPrompt(t *testing.T) {
	args := buildArgs("hello world", "/tmp/session.jsonl")
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
	sessionID := "4d0ce91b-0856-44e2-b0d7-7765d824bba3"
	inv, err := NewInvoker("true", "http://localhost:8080", nil)
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

func TestInvoker_Run_NoSessionIDOutput(t *testing.T) {
	inv, err := NewInvoker("true", "http://localhost:8080", nil)
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

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "pi-log-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString(content)
	return f.Name()
}
