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

func TestBuildArgs_StripsFrontmatter(t *testing.T) {
	prompt := "---\nmessage_id: abc\ntask_id: xyz\n---\n\nhello world"
	args := buildArgs(prompt, "/tmp/session.jsonl")
	want := []string{"--mode", "json", "--session", "/tmp/session.jsonl", "-p", "hello world"}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestStripFrontmatter(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no frontmatter",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "with frontmatter",
			input: "---\nmessage_id: abc\ntask_id: xyz\n---\n\nhello world",
			want:  "hello world",
		},
		{
			name:  "frontmatter with single field",
			input: "---\nmessage_id: abc\n---\n\ndo something",
			want:  "do something",
		},
		{
			name:  "unclosed frontmatter passes through",
			input: "---\nmessage_id: abc\n",
			want:  "---\nmessage_id: abc\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripFrontmatter(tc.input)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
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

func TestInvoker_Run_EmitsSessionID(t *testing.T) {
	// Use a dummy binary that exits immediately.
	inv := NewInvoker("true", "http://localhost:8080", nil)
	logPath := filepath.Join(t.TempDir(), "pi.log")

	_, ch, err := inv.Run(context.Background(), t.TempDir(), "hello", ai.RunOptions{}, logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var gotSessionID bool
	for out := range ch {
		if out.Type == ai.OutputSessionID && out.Content != "" {
			gotSessionID = true
		}
	}
	if !gotSessionID {
		t.Error("expected OutputSessionID event")
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
