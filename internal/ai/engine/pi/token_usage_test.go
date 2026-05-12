package pi_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/engine/pi"
)

func writePiTempFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
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
	collector := pi.NewBackendAt("", nil, sessionsDir)

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

	usages, err := pi.NewBackendAt("", nil, sessionsDir).Collect(context.Background(), sessionID)
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
	_, err := pi.NewBackendAt("", nil, dir).Collect(context.Background(), "nonexistent-session")
	if !errors.Is(err, ai.ErrSessionDataNotFound) {
		t.Fatalf("expected ErrSessionDataNotFound, got %v", err)
	}
}

func TestPiCollector_Collect_DirectoryNotFound(t *testing.T) {
	_, err := pi.NewBackendAt("", nil, t.TempDir()+"/missing").Collect(context.Background(), "nonexistent-session")
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
	usages, err := pi.NewBackendAt("", nil, sessionsDir).Collect(context.Background(), sessionID)
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
