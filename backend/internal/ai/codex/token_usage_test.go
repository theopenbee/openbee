package codex_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/codex"
)

// writeCodexTempFile creates a file at dir/name with the given content.
func writeCodexTempFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
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

	collector := codex.NewCollectorAt(mappingDir, codexBase)
	usages, err := collector.Collect(context.Background(), "openbee-sess-1")
	require.NoError(t, err)

	byModel := map[string]ai.TokenUsage{}
	for _, u := range usages {
		byModel[u.Model] = u
	}

	gpt4o := byModel["gpt-4o"]
	assert.Equal(t, int64(300), gpt4o.InputTokens)
	assert.Equal(t, int64(130), gpt4o.OutputTokens)
	assert.Equal(t, int64(30), gpt4o.CacheReadTokens)
	assert.Equal(t, int64(0), gpt4o.CacheCreationTokens)

	o1mini := byModel["o1-mini"]
	assert.Equal(t, int64(300), o1mini.InputTokens)
}

func TestCodexCollector_Collect(t *testing.T) {
	cases := []struct {
		name       string
		sessionID  string
		realSessID string
		jsonl      string
		check      func(t *testing.T, usages []ai.TokenUsage)
	}{
		{
			name:       "DeltaFromTotalTokenUsage",
			sessionID:  "openbee-sess-2",
			realSessID: "codex-real-sess-2",
			jsonl: `{"type":"turn_context","payload":{"model":"gpt-4o"}}
{"type":"event_msg","info":{"total_token_usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":10}}}
{"type":"event_msg","info":{"total_token_usage":{"input_tokens":250,"output_tokens":120,"cached_input_tokens":25}}}
`,
			check: func(t *testing.T, usages []ai.TokenUsage) {
				require.Len(t, usages, 1)
				assert.Equal(t, int64(250), usages[0].InputTokens)
				assert.Equal(t, int64(120), usages[0].OutputTokens)
			},
		},
		{
			name:       "LegacyTopLevelInfo",
			sessionID:  "openbee-sess-legacy",
			realSessID: "codex-real-sess-legacy",
			jsonl: `{"type":"turn_context","payload":{"model":"gpt-4o"}}
{"type":"event_msg","info":{"last_token_usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":10}}}
`,
			check: func(t *testing.T, usages []ai.TokenUsage) {
				require.Len(t, usages, 1)
				require.Equal(t, int64(100), usages[0].InputTokens)
				require.Equal(t, int64(50), usages[0].OutputTokens)
				require.Equal(t, int64(10), usages[0].CacheReadTokens)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			mappingDir := filepath.Join(base, "mapping")
			codexBase := filepath.Join(base, "codex")
			os.MkdirAll(mappingDir, 0755)
			os.MkdirAll(filepath.Join(codexBase, "sessions"), 0755)

			os.WriteFile(filepath.Join(mappingDir, tc.sessionID), []byte(tc.realSessID), 0644)
			writeCodexTempFile(t, filepath.Join(codexBase, "sessions"), tc.realSessID+".jsonl", tc.jsonl)

			collector := codex.NewCollectorAt(mappingDir, codexBase)
			usages, err := collector.Collect(context.Background(), tc.sessionID)
			require.NoError(t, err)
			tc.check(t, usages)
		})
	}
}

func TestCodexCollector_Collect_MappingFileNotFound(t *testing.T) {
	mappingDir := t.TempDir()
	codexBase := t.TempDir()
	collector := codex.NewCollectorAt(mappingDir, codexBase)
	_, err := collector.Collect(context.Background(), "nonexistent-session")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ai.ErrSessionDataNotFound)
}
