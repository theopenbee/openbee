package tokenstat_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/theopenbee/openbee/internal/tokenstat"
)

// writeTempFile creates a file at dir/name with content. Used by all parser tests.
func writeTempFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestClaudeParser_Parse_AggregatesByModel(t *testing.T) {
	base := t.TempDir()
	writeTempFile(t, base, "projects/project-a/test-session.jsonl", `{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":20,"cache_read_input_tokens":10}}}
{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":5}}}
{"message":{"model":"claude-3-opus","usage":{"input_tokens":300,"output_tokens":150}}}
{"timestamp":"2025-01-01T00:00:00Z"}
`)
	t.Setenv("CLAUDE_CONFIG_DIR", base)
	parser := tokenstat.NewClaudeParser()

	usages, err := parser.Parse("test-session")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	byModel := map[string]tokenstat.SessionTokenUsage{}
	for _, u := range usages {
		byModel[u.Model] = u
	}

	sonnet := byModel["claude-3-5-sonnet"]
	if sonnet.InputTokens != 300 {
		t.Errorf("sonnet InputTokens: want 300, got %d", sonnet.InputTokens)
	}
	if sonnet.OutputTokens != 150 {
		t.Errorf("sonnet OutputTokens: want 150, got %d", sonnet.OutputTokens)
	}
	if sonnet.CacheCreationTokens != 20 {
		t.Errorf("sonnet CacheCreationTokens: want 20, got %d", sonnet.CacheCreationTokens)
	}
	if sonnet.CacheReadTokens != 15 {
		t.Errorf("sonnet CacheReadTokens: want 15, got %d", sonnet.CacheReadTokens)
	}
	if sonnet.AgentType != "claude" {
		t.Errorf("sonnet AgentType: want claude, got %s", sonnet.AgentType)
	}

	opus := byModel["claude-3-opus"]
	if opus.InputTokens != 300 {
		t.Errorf("opus InputTokens: want 300, got %d", opus.InputTokens)
	}
}

func TestClaudeParser_Parse_FastSpeedSuffix(t *testing.T) {
	base := t.TempDir()
	writeTempFile(t, base, "projects/project-a/fast-session.jsonl",
		`{"message":{"model":"claude-3-5-sonnet","speed":"fast","usage":{"input_tokens":100,"output_tokens":50}}}`+"\n")
	t.Setenv("CLAUDE_CONFIG_DIR", base)

	usages, err := tokenstat.NewClaudeParser().Parse("fast-session")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].Model != "claude-3-5-sonnet-fast" {
		t.Errorf("Model: want claude-3-5-sonnet-fast, got %s", usages[0].Model)
	}
}

func TestClaudeParser_Parse_FileNotFound(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	_, err := tokenstat.NewClaudeParser().Parse("nonexistent-session")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}
