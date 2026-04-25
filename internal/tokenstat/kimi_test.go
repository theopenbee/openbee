package tokenstat_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/theopenbee/openbee/internal/tokenstat"
)

func makeKimiSessionFile(t *testing.T, home, sessionID, content string) {
	t.Helper()
	dir := filepath.Join(home, ".kimi", "sessions", "bucket-01", sessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wire.jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("write wire.jsonl: %v", err)
	}
}

func TestKimiParser_Parse_TakesLastStatusUpdate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	makeKimiSessionFile(t, home, "sess-abc", `{"message":{"type":"Other","payload":{}}}
{"message":{"type":"StatusUpdate","payload":{"token_usage":{"input_other":100,"output":50,"input_cache_read":200,"input_cache_creation":10}}}}
{"message":{"type":"StatusUpdate","payload":{"token_usage":{"input_other":446,"output":70,"input_cache_read":16384,"input_cache_creation":0}}}}
`)

	usages, err := tokenstat.NewKimiParser().Parse("sess-abc")
	if err != nil {
		t.Fatalf("Parse: %v", err)
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
	if u.AgentType != "kimi" {
		t.Errorf("AgentType: want kimi, got %s", u.AgentType)
	}
	if u.SessionID != "sess-abc" {
		t.Errorf("SessionID: want sess-abc, got %s", u.SessionID)
	}
}

func TestKimiParser_Parse_FileNotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, err := tokenstat.NewKimiParser().Parse("nonexistent-session")
	if !errors.Is(err, tokenstat.ErrSessionDataNotFound) {
		t.Fatalf("expected ErrSessionDataNotFound, got %v", err)
	}
}

func TestKimiParser_Parse_NoStatusUpdate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	makeKimiSessionFile(t, home, "sess-empty", `{"message":{"type":"Other","payload":{}}}
{"message":{"type":"Progress","payload":{}}}
`)

	_, err := tokenstat.NewKimiParser().Parse("sess-empty")
	if !errors.Is(err, tokenstat.ErrSessionDataNotFound) {
		t.Fatalf("expected ErrSessionDataNotFound, got %v", err)
	}
}

func TestKimiParser_Parse_ZeroTokens(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	makeKimiSessionFile(t, home, "sess-zero", `{"message":{"type":"StatusUpdate","payload":{"token_usage":{"input_other":0,"output":0,"input_cache_read":0,"input_cache_creation":0}}}}
`)

	usages, err := tokenstat.NewKimiParser().Parse("sess-zero")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	u := usages[0]
	if u.InputTokens != 0 || u.OutputTokens != 0 {
		t.Errorf("expected all zero tokens, got %+v", u)
	}
}
