package kimi

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "kimi-log-*.jsonl")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	f.Close()
	return f.Name()
}

func TestBuildArgs(t *testing.T) {
	args := buildArgs("550e8400-e29b-41d4-a716-446655440000", nil)
	want := []string{
		"--session=550e8400-e29b-41d4-a716-446655440000",
		"--yolo",
		"--output-format=stream-json",
		"--print",
	}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestExtractResultFromLog_StringContent(t *testing.T) {
	log := `{"role":"user","content":"hello"}
{"role":"assistant","content":"world"}
`
	path := writeTemp(t, log)
	got := ExtractResultFromLog(path)
	if got != "world" {
		t.Errorf("got %q, want %q", got, "world")
	}
}

func TestExtractResultFromLog_ArrayContent(t *testing.T) {
	log := `{"role":"assistant","content":[{"type":"text","text":"array answer"}]}
`
	path := writeTemp(t, log)
	got := ExtractResultFromLog(path)
	if got != "array answer" {
		t.Errorf("got %q, want %q", got, "array answer")
	}
}

func TestExtractResultFromLog_ArrayContentFirstTextBlock(t *testing.T) {
	log := `{"role":"assistant","content":[{"type":"tool_use","id":"tc_1"},{"type":"text","text":"after tool"}]}
`
	path := writeTemp(t, log)
	got := ExtractResultFromLog(path)
	if got != "after tool" {
		t.Errorf("got %q, want %q", got, "after tool")
	}
}

func TestExtractResultFromLog_LastAssistantWins(t *testing.T) {
	log := `{"role":"assistant","content":"first"}
{"role":"tool","tool_call_id":"tc_1","content":"result"}
{"role":"assistant","content":"last"}
`
	path := writeTemp(t, log)
	got := ExtractResultFromLog(path)
	if got != "last" {
		t.Errorf("got %q, want %q", got, "last")
	}
}

func TestExtractResultFromLog_Empty(t *testing.T) {
	path := writeTemp(t, "")
	got := ExtractResultFromLog(path)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractResultFromLog_NoAssistant(t *testing.T) {
	log := `{"role":"user","content":"hi"}
{"role":"tool","tool_call_id":"x","content":"done"}
`
	path := writeTemp(t, log)
	got := ExtractResultFromLog(path)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractResultFromLog_MissingFile(t *testing.T) {
	got := ExtractResultFromLog(filepath.Join(t.TempDir(), "nonexistent.jsonl"))
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
