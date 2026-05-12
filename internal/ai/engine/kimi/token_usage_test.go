package kimi_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/engine/kimi"
)

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

func TestKimiCollector_Collect_TakesLastStatusUpdate(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, ".kimi", "sessions")
	makeKimiSessionFile(t, sessionsDir, "sess-abc", `{"message":{"type":"Other","payload":{}}}
{"message":{"type":"StatusUpdate","payload":{"token_usage":{"input_other":100,"output":50,"input_cache_read":200,"input_cache_creation":10}}}}
{"message":{"type":"StatusUpdate","payload":{"token_usage":{"input_other":446,"output":70,"input_cache_read":16384,"input_cache_creation":0}}}}
`)

	collector := kimi.NewBackendAt("", nil, sessionsDir)
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

	collector := kimi.NewBackendAt("", nil, sessionsDir)
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

	collector := kimi.NewBackendAt("", nil, sessionsDir)
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

	collector := kimi.NewBackendAt("", nil, sessionsDir)
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
