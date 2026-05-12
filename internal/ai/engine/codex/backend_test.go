package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/ai"
)

func writeCodexTempFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile %s/%s: %v", dir, name, err)
	}
}

func TestBuildArgs_NewSession(t *testing.T) {
	args := buildArgs("", false, "", nil)
	want := []string{"exec", "-", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildArgs_ResumeWithID(t *testing.T) {
	args := buildArgs("sess-123", true, "", nil)
	want := []string{"exec", "resume", "sess-123", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildArgs_ResumeWithIDAndPrompt(t *testing.T) {
	args := buildArgs("sess-123", true, "do something", nil)
	want := []string{"exec", "resume", "sess-123", "--json", "--dangerously-bypass-approvals-and-sandbox", "do something"}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestExtractSessionID(t *testing.T) {
	jsonStream := `{"type":"thread.started","thread_id":"019d7293-0a51-71e0-b634-02183839d7b2"}
{"type":"turn.started"}
{"type":"turn.completed","usage":{}}
`
	id := extractSessionID(strings.NewReader(jsonStream))
	if id != "019d7293-0a51-71e0-b634-02183839d7b2" {
		t.Errorf("got %q, want 019d7293-0a51-71e0-b634-02183839d7b2", id)
	}
}

func TestBuildArgs_ResumeUsesThreadID(t *testing.T) {
	args := buildArgs("thread-xyz-from-store", true, "follow up", nil)
	want := []string{"exec", "resume", "thread-xyz-from-store", "--json", "--dangerously-bypass-approvals-and-sandbox", "follow up"}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestExtractResultFromLog(t *testing.T) {
	jsonStream := `{"type":"thread.started","thread_id":"abc"}
{"type":"item.completed","item":{"type":"agent_message","text":"hello world"}}
{"type":"turn.completed","usage":{}}
`
	tmpFile := t.TempDir() + "/test.log"
	if err := os.WriteFile(tmpFile, []byte(jsonStream), 0o644); err != nil {
		t.Fatal(err)
	}
	result := (&Backend{}).Extract(tmpFile)
	if result != "hello world" {
		t.Errorf("got %q, want %q", result, "hello world")
	}
}

func TestCodexCollector_Collect_WithLastTokenUsage(t *testing.T) {
	base := t.TempDir()
	mappingDir := filepath.Join(base, "mapping")
	codexBase := filepath.Join(base, "codex")
	os.MkdirAll(mappingDir, 0755)
	os.MkdirAll(filepath.Join(codexBase, "sessions", "2026", "04", "23"), 0755)

	os.WriteFile(filepath.Join(mappingDir, "openbee-sess-1"), []byte("codex-real-sess-1\n"), 0644)
	writeCodexTempFile(t, filepath.Join(codexBase, "sessions", "2026", "04", "23"), "rollout-2026-04-23T01-02-03-codex-real-sess-1.jsonl", `{"type":"turn_context","payload":{"model":"gpt-4o"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":20},"total_token_usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":20}}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":200,"output_tokens":80,"cached_input_tokens":10},"total_token_usage":{"input_tokens":300,"output_tokens":130,"cached_input_tokens":30}}}}
{"type":"turn_context","payload":{"model":"o1-mini"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":300,"output_tokens":100,"cached_input_tokens":0},"total_token_usage":{"input_tokens":600,"output_tokens":230,"cached_input_tokens":30}}}}
`)

	collector := NewBackendAt("", nil, nil, mappingDir, codexBase)
	usages, err := collector.Collect(context.Background(), "openbee-sess-1")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	byModel := map[string]ai.TokenUsage{}
	for _, u := range usages {
		byModel[u.Model] = u
	}

	gpt4o := byModel["gpt-4o"]
	if gpt4o.InputTokens != 300 {
		t.Errorf("gpt-4o InputTokens: want 300, got %d", gpt4o.InputTokens)
	}
	if gpt4o.OutputTokens != 130 {
		t.Errorf("gpt-4o OutputTokens: want 130, got %d", gpt4o.OutputTokens)
	}
	if gpt4o.CacheReadTokens != 30 {
		t.Errorf("gpt-4o CacheReadTokens: want 30, got %d", gpt4o.CacheReadTokens)
	}
	if gpt4o.CacheCreationTokens != 0 {
		t.Errorf("gpt-4o CacheCreationTokens: want 0, got %d", gpt4o.CacheCreationTokens)
	}

	o1mini := byModel["o1-mini"]
	if o1mini.InputTokens != 300 {
		t.Errorf("o1-mini InputTokens: want 300, got %d", o1mini.InputTokens)
	}
}

func TestCodexCollector_Collect_DeltaFromTotalTokenUsage(t *testing.T) {
	base := t.TempDir()
	mappingDir := filepath.Join(base, "mapping")
	codexBase := filepath.Join(base, "codex")
	os.MkdirAll(mappingDir, 0755)
	os.MkdirAll(filepath.Join(codexBase, "sessions"), 0755)

	os.WriteFile(filepath.Join(mappingDir, "openbee-sess-2"), []byte("codex-real-sess-2"), 0644)
	writeCodexTempFile(t, filepath.Join(codexBase, "sessions"), "codex-real-sess-2.jsonl", `{"type":"turn_context","payload":{"model":"gpt-4o"}}
{"type":"event_msg","info":{"total_token_usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":10}}}
{"type":"event_msg","info":{"total_token_usage":{"input_tokens":250,"output_tokens":120,"cached_input_tokens":25}}}
`)

	collector := NewBackendAt("", nil, nil, mappingDir, codexBase)
	usages, err := collector.Collect(context.Background(), "openbee-sess-2")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].InputTokens != 250 {
		t.Errorf("InputTokens: want 250, got %d", usages[0].InputTokens)
	}
	if usages[0].OutputTokens != 120 {
		t.Errorf("OutputTokens: want 120, got %d", usages[0].OutputTokens)
	}
}

func TestCodexCollector_Collect_LegacyTopLevelInfo(t *testing.T) {
	base := t.TempDir()
	mappingDir := filepath.Join(base, "mapping")
	codexBase := filepath.Join(base, "codex")
	os.MkdirAll(mappingDir, 0755)
	os.MkdirAll(filepath.Join(codexBase, "sessions"), 0755)

	os.WriteFile(filepath.Join(mappingDir, "openbee-sess-legacy"), []byte("codex-real-sess-legacy"), 0644)
	writeCodexTempFile(t, filepath.Join(codexBase, "sessions"), "codex-real-sess-legacy.jsonl", `{"type":"turn_context","payload":{"model":"gpt-4o"}}
{"type":"event_msg","info":{"last_token_usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":10}}}
`)

	collector := NewBackendAt("", nil, nil, mappingDir, codexBase)
	usages, err := collector.Collect(context.Background(), "openbee-sess-legacy")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].InputTokens != 100 || usages[0].OutputTokens != 50 || usages[0].CacheReadTokens != 10 {
		t.Fatalf("unexpected usage: %+v", usages[0])
	}
}

func TestCodexCollector_Collect_MappingFileNotFound(t *testing.T) {
	mappingDir := t.TempDir()
	codexBase := t.TempDir()
	collector := NewBackendAt("", nil, nil, mappingDir, codexBase)
	_, err := collector.Collect(context.Background(), "nonexistent-session")
	if err == nil {
		t.Error("expected error for missing mapping file, got nil")
	}
	if !errors.Is(err, ai.ErrSessionDataNotFound) {
		t.Errorf("expected ErrSessionDataNotFound, got: %v", err)
	}
}
