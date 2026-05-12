package kimi

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

func makeKimiSessionFile(t *testing.T, sessionsDir, sessionID, content string) {
	t.Helper()
	dir := filepath.Join(sessionsDir, "bucket-01", sessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wire.jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("write wire.jsonl: %v", err)
	}
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
	got := (&Backend{}).Extract(path)
	if got != "world" {
		t.Errorf("got %q, want %q", got, "world")
	}
}

func TestExtractResultFromLog_ArrayContent(t *testing.T) {
	log := `{"role":"assistant","content":[{"type":"text","text":"array answer"}]}
`
	path := writeTemp(t, log)
	got := (&Backend{}).Extract(path)
	if got != "array answer" {
		t.Errorf("got %q, want %q", got, "array answer")
	}
}

func TestExtractResultFromLog_ArrayContentFirstTextBlock(t *testing.T) {
	log := `{"role":"assistant","content":[{"type":"tool_use","id":"tc_1"},{"type":"text","text":"after tool"}]}
`
	path := writeTemp(t, log)
	got := (&Backend{}).Extract(path)
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
	got := (&Backend{}).Extract(path)
	if got != "last" {
		t.Errorf("got %q, want %q", got, "last")
	}
}

func TestExtractResultFromLog_Empty(t *testing.T) {
	path := writeTemp(t, "")
	got := (&Backend{}).Extract(path)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractResultFromLog_NoAssistant(t *testing.T) {
	log := `{"role":"user","content":"hi"}
{"role":"tool","tool_call_id":"x","content":"done"}
`
	path := writeTemp(t, log)
	got := (&Backend{}).Extract(path)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractResultFromLog_MissingFile(t *testing.T) {
	got := (&Backend{}).Extract(filepath.Join(t.TempDir(), "nonexistent.jsonl"))
	if got != "" {
		t.Errorf("got %q, want %q", got, "")
	}
}

func TestKimiCollector_Collect_TakesLastStatusUpdate(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, ".kimi", "sessions")
	makeKimiSessionFile(t, sessionsDir, "sess-abc", `{"message":{"type":"Other","payload":{}}}
{"message":{"type":"StatusUpdate","payload":{"token_usage":{"input_other":100,"output":50,"input_cache_read":200,"input_cache_creation":10}}}}
{"message":{"type":"StatusUpdate","payload":{"token_usage":{"input_other":446,"output":70,"input_cache_read":16384,"input_cache_creation":0}}}}
`)

	collector := NewBackendAt("", nil, sessionsDir)
	usages, err := collector.Collect(context.Background(), "sess-abc")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	u := usages[0]
	if u.InputTokens != 446 {
		t.Errorf("InputTokens: want 446, got %d", u.InputTokens)
	}
	if u.OutputTokens != 70 {
		t.Errorf("OutputTokens: want 70, got %d", u.OutputTokens)
	}
	if u.CacheReadTokens != 16384 {
		t.Errorf("CacheReadTokens: want 16384, got %d", u.CacheReadTokens)
	}
	if u.CacheCreationTokens != 0 {
		t.Errorf("CacheCreationTokens: want 0, got %d", u.CacheCreationTokens)
	}
	if u.Model != "kimi" {
		t.Errorf("Model: want kimi, got %s", u.Model)
	}
}

func TestKimiCollector_Collect_FileNotFound(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, ".kimi", "sessions")

	collector := NewBackendAt("", nil, sessionsDir)
	_, err := collector.Collect(context.Background(), "nonexistent-session")
	if !errors.Is(err, ai.ErrSessionDataNotFound) {
		t.Fatalf("expected ErrSessionDataNotFound, got %v", err)
	}
}

func TestKimiCollector_Collect_NoStatusUpdate(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, ".kimi", "sessions")
	makeKimiSessionFile(t, sessionsDir, "sess-empty", `{"message":{"type":"Other","payload":{}}}
{"message":{"type":"Progress","payload":{}}}
`)

	collector := NewBackendAt("", nil, sessionsDir)
	_, err := collector.Collect(context.Background(), "sess-empty")
	if !errors.Is(err, ai.ErrSessionDataNotFound) {
		t.Fatalf("expected ErrSessionDataNotFound, got %v", err)
	}
}

func TestKimiCollector_Collect_ZeroTokens(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, ".kimi", "sessions")
	makeKimiSessionFile(t, sessionsDir, "sess-zero", `{"message":{"type":"StatusUpdate","payload":{"token_usage":{"input_other":0,"output":0,"input_cache_read":0,"input_cache_creation":0}}}}
`)

	collector := NewBackendAt("", nil, sessionsDir)
	usages, err := collector.Collect(context.Background(), "sess-zero")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	u := usages[0]
	if u.InputTokens != 0 || u.OutputTokens != 0 {
		t.Errorf("expected all zero tokens, got %+v", u)
	}
}
