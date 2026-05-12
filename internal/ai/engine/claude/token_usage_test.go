package claude_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/engine/claude"
)

func writeClaudeTempFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestClaudeCollector_Collect_AggregatesByModel(t *testing.T) {
	base := t.TempDir()
	writeClaudeTempFile(t, base, "projects/project-a/test-session.jsonl", `{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":20,"cache_read_input_tokens":10}}}
{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":5}}}
{"message":{"model":"claude-3-opus","usage":{"input_tokens":300,"output_tokens":150}}}
{"timestamp":"2025-01-01T00:00:00Z"}
`)
	t.Setenv("CLAUDE_CONFIG_DIR", base)
	collector := claude.NewBackend("", nil)

	usages, err := collector.Collect(context.Background(), "test-session")
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
	if sonnet.OutputTokens != 150 {
		t.Errorf("sonnet OutputTokens: want 150, got %d", sonnet.OutputTokens)
	}
	if sonnet.CacheCreationTokens != 20 {
		t.Errorf("sonnet CacheCreationTokens: want 20, got %d", sonnet.CacheCreationTokens)
	}
	if sonnet.CacheReadTokens != 15 {
		t.Errorf("sonnet CacheReadTokens: want 15, got %d", sonnet.CacheReadTokens)
	}

	opus := byModel["claude-3-opus"]
	if opus.InputTokens != 300 {
		t.Errorf("opus InputTokens: want 300, got %d", opus.InputTokens)
	}
}

func TestClaudeCollector_Collect_FastSpeedSuffix(t *testing.T) {
	base := t.TempDir()
	writeClaudeTempFile(t, base, "projects/project-a/fast-session.jsonl",
		`{"message":{"model":"claude-3-5-sonnet","speed":"fast","usage":{"input_tokens":100,"output_tokens":50}}}`+"\n")
	t.Setenv("CLAUDE_CONFIG_DIR", base)

	usages, err := claude.NewBackend("", nil).Collect(context.Background(), "fast-session")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].Model != "claude-3-5-sonnet-fast" {
		t.Errorf("Model: want claude-3-5-sonnet-fast, got %s", usages[0].Model)
	}
}

func TestClaudeCollector_Collect_SkipsSyntheticModel(t *testing.T) {
	base := t.TempDir()
	writeClaudeTempFile(t, base, "projects/project-a/syn-session.jsonl",
		`{"message":{"model":"<synthetic>","usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`+"\n"+
			`{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":100,"output_tokens":50}}}`+"\n")
	t.Setenv("CLAUDE_CONFIG_DIR", base)

	usages, err := claude.NewBackend("", nil).Collect(context.Background(), "syn-session")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].Model != "claude-3-5-sonnet" {
		t.Errorf("Model: want claude-3-5-sonnet, got %s", usages[0].Model)
	}
}

func TestClaudeCollector_Collect_FileNotFound(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	_, err := claude.NewBackend("", nil).Collect(context.Background(), "nonexistent-session")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
	if !errors.Is(err, ai.ErrSessionDataNotFound) {
		t.Errorf("expected ErrSessionDataNotFound, got: %v", err)
	}
}
