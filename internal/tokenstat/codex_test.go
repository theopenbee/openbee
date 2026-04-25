package tokenstat_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/theopenbee/openbee/internal/tokenstat"
)

func TestCodexParser_Parse_WithLastTokenUsage(t *testing.T) {
	base := t.TempDir()
	mappingDir := filepath.Join(base, "mapping")
	codexBase := filepath.Join(base, "codex")
	os.MkdirAll(mappingDir, 0755)
	os.MkdirAll(filepath.Join(codexBase, "sessions", "2026", "04", "23"), 0755)

	os.WriteFile(filepath.Join(mappingDir, "openbee-sess-1"), []byte("codex-real-sess-1\n"), 0644)
	writeTempFile(t, filepath.Join(codexBase, "sessions", "2026", "04", "23"), "rollout-2026-04-23T01-02-03-codex-real-sess-1.jsonl", `{"type":"turn_context","payload":{"model":"gpt-4o"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":20},"total_token_usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":20}}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":200,"output_tokens":80,"cached_input_tokens":10},"total_token_usage":{"input_tokens":300,"output_tokens":130,"cached_input_tokens":30}}}}
{"type":"turn_context","payload":{"model":"o1-mini"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":300,"output_tokens":100,"cached_input_tokens":0},"total_token_usage":{"input_tokens":600,"output_tokens":230,"cached_input_tokens":30}}}}
`)
	t.Setenv("CODEX_HOME", codexBase)
	parser := tokenstat.NewCodexParser(mappingDir)

	usages, err := parser.Parse("openbee-sess-1")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	byModel := map[string]tokenstat.SessionTokenUsage{}
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
	if gpt4o.AgentType != "codex" {
		t.Errorf("gpt-4o AgentType: want codex, got %s", gpt4o.AgentType)
	}

	o1mini := byModel["o1-mini"]
	if o1mini.InputTokens != 300 {
		t.Errorf("o1-mini InputTokens: want 300, got %d", o1mini.InputTokens)
	}
}

func TestCodexParser_Parse_DeltaFromTotalTokenUsage(t *testing.T) {
	base := t.TempDir()
	mappingDir := filepath.Join(base, "mapping")
	codexBase := filepath.Join(base, "codex")
	os.MkdirAll(mappingDir, 0755)
	os.MkdirAll(filepath.Join(codexBase, "sessions"), 0755)

	os.WriteFile(filepath.Join(mappingDir, "openbee-sess-2"), []byte("codex-real-sess-2"), 0644)
	writeTempFile(t, filepath.Join(codexBase, "sessions"), "codex-real-sess-2.jsonl", `{"type":"turn_context","payload":{"model":"gpt-4o"}}
{"type":"event_msg","info":{"total_token_usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":10}}}
{"type":"event_msg","info":{"total_token_usage":{"input_tokens":250,"output_tokens":120,"cached_input_tokens":25}}}
`)
	t.Setenv("CODEX_HOME", codexBase)

	usages, err := tokenstat.NewCodexParser(mappingDir).Parse("openbee-sess-2")
	if err != nil {
		t.Fatalf("Parse: %v", err)
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

func TestCodexParser_Parse_LegacyTopLevelInfo(t *testing.T) {
	base := t.TempDir()
	mappingDir := filepath.Join(base, "mapping")
	codexBase := filepath.Join(base, "codex")
	os.MkdirAll(mappingDir, 0755)
	os.MkdirAll(filepath.Join(codexBase, "sessions"), 0755)

	os.WriteFile(filepath.Join(mappingDir, "openbee-sess-legacy"), []byte("codex-real-sess-legacy"), 0644)
	writeTempFile(t, filepath.Join(codexBase, "sessions"), "codex-real-sess-legacy.jsonl", `{"type":"turn_context","payload":{"model":"gpt-4o"}}
{"type":"event_msg","info":{"last_token_usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":10}}}
`)
	t.Setenv("CODEX_HOME", codexBase)

	usages, err := tokenstat.NewCodexParser(mappingDir).Parse("openbee-sess-legacy")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].InputTokens != 100 || usages[0].OutputTokens != 50 || usages[0].CacheReadTokens != 10 {
		t.Fatalf("unexpected usage: %+v", usages[0])
	}
}

func TestCodexParser_Parse_MappingFileNotFound(t *testing.T) {
	mappingDir := t.TempDir()
	t.Setenv("CODEX_HOME", t.TempDir())
	_, err := tokenstat.NewCodexParser(mappingDir).Parse("nonexistent-session")
	if err == nil {
		t.Error("expected error for missing mapping file, got nil")
	}
}
