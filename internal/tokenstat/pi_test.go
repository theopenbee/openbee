package tokenstat_test

import (
	"errors"
	"testing"

	"github.com/theopenbee/openbee/internal/tokenstat"
)

func TestPiParser_Parse_AggregatesByModel(t *testing.T) {
	sessionsDir := t.TempDir()
	sessionID := "pi-sess-abc123"

	writeTempFile(t, sessionsDir, "20250101_"+sessionID+".jsonl", `{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":100,"output":50,"cacheWrite":10,"cacheRead":5}}}
{"type":"message","message":{"role":"user","content":"hello"}}
{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":200,"output":80,"cacheWrite":0,"cacheRead":15}}}
{"type":"message","message":{"role":"assistant","model":"claude-3-opus","usage":{"input":300,"output":100,"cacheWrite":5,"cacheRead":0}}}
{"type":"other","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":999,"output":999}}}
`)
	t.Setenv("PI_AGENT_DIR", sessionsDir)
	parser := tokenstat.NewPiParser()

	usages, err := parser.Parse(sessionID)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	byModel := map[string]tokenstat.SessionTokenUsage{}
	for _, u := range usages {
		byModel[u.Model] = u
	}

	sonnet := byModel["[pi]claude-3-5-sonnet"]
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
	if sonnet.AgentType != "pi" {
		t.Errorf("sonnet AgentType: want pi, got %s", sonnet.AgentType)
	}

	opus := byModel["[pi]claude-3-opus"]
	if opus.InputTokens != 300 {
		t.Errorf("opus InputTokens: want 300, got %d", opus.InputTokens)
	}
}

func TestPiParser_Parse_SkipsNonAssistantAndWrongType(t *testing.T) {
	sessionsDir := t.TempDir()
	sessionID := "skip-test"

	writeTempFile(t, sessionsDir, "ts_"+sessionID+".jsonl", `{"type":"message","message":{"role":"user","model":"claude-3-5-sonnet","usage":{"input":999,"output":999}}}
{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":100,"output":50}}}
{"type":"other","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":999,"output":999}}}
`)
	t.Setenv("PI_AGENT_DIR", sessionsDir)

	usages, err := tokenstat.NewPiParser().Parse(sessionID)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage (only assistant+message), got %d", len(usages))
	}
	if usages[0].InputTokens != 100 {
		t.Errorf("InputTokens: want 100, got %d", usages[0].InputTokens)
	}
}

func TestPiParser_Parse_FileNotFound(t *testing.T) {
	t.Setenv("PI_AGENT_DIR", t.TempDir())
	_, err := tokenstat.NewPiParser().Parse("nonexistent-session")
	if !errors.Is(err, tokenstat.ErrSessionDataNotFound) {
		t.Fatalf("expected ErrSessionDataNotFound, got %v", err)
	}
}

func TestPiParser_Parse_DirectoryNotFound(t *testing.T) {
	t.Setenv("PI_AGENT_DIR", t.TempDir()+"/missing")
	_, err := tokenstat.NewPiParser().Parse("nonexistent-session")
	if !errors.Is(err, tokenstat.ErrSessionDataNotFound) {
		t.Fatalf("expected ErrSessionDataNotFound, got %v", err)
	}
}

func TestPiParser_Parse_UsesOpenbeeDefaultSessionsDir(t *testing.T) {
	home := t.TempDir()
	sessionID := "default-dir-session"
	t.Setenv("HOME", home)
	t.Setenv("PI_AGENT_DIR", "")

	writeTempFile(t, home, ".openbee/.pi/sessions/20250101_"+sessionID+".jsonl", `{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":100,"output":50}}}`)

	usages, err := tokenstat.NewPiParser().Parse(sessionID)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].InputTokens != 100 || usages[0].OutputTokens != 50 {
		t.Fatalf("unexpected usage: %+v", usages[0])
	}
}
