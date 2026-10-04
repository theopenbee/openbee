package claude_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/claude"
)

func writeClaudeTempFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
}

func TestClaudeCollector_Collect_AggregatesByModel(t *testing.T) {
	base := t.TempDir()
	writeClaudeTempFile(t, base, "projects/project-a/test-session.jsonl", `{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":20,"cache_read_input_tokens":10}}}
{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":5}}}
{"message":{"model":"claude-3-opus","usage":{"input_tokens":300,"output_tokens":150}}}
{"timestamp":"2025-01-01T00:00:00Z"}
`)
	t.Setenv("CLAUDE_CONFIG_DIR", base)
	collector := claude.NewCollector()

	usages, err := collector.Collect(context.Background(), "test-session")
	require.NoError(t, err)

	byModel := map[string]ai.TokenUsage{}
	for _, u := range usages {
		byModel[u.Model] = u
	}

	sonnet := byModel["claude-3-5-sonnet"]
	assert.Equal(t, int64(300), sonnet.InputTokens)
	assert.Equal(t, int64(150), sonnet.OutputTokens)
	assert.Equal(t, int64(20), sonnet.CacheCreationTokens)
	assert.Equal(t, int64(15), sonnet.CacheReadTokens)

	opus := byModel["claude-3-opus"]
	assert.Equal(t, int64(300), opus.InputTokens)
}

func TestClaudeCollector_Collect_FastSpeedSuffix(t *testing.T) {
	base := t.TempDir()
	writeClaudeTempFile(t, base, "projects/project-a/fast-session.jsonl",
		`{"message":{"model":"claude-3-5-sonnet","speed":"fast","usage":{"input_tokens":100,"output_tokens":50}}}`+"\n")
	t.Setenv("CLAUDE_CONFIG_DIR", base)

	usages, err := claude.NewCollector().Collect(context.Background(), "fast-session")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.Equal(t, "claude-3-5-sonnet-fast", usages[0].Model)
}

func TestClaudeCollector_Collect_SkipsSyntheticModel(t *testing.T) {
	base := t.TempDir()
	writeClaudeTempFile(t, base, "projects/project-a/syn-session.jsonl",
		`{"message":{"model":"<synthetic>","usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`+"\n"+
			`{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":100,"output_tokens":50}}}`+"\n")
	t.Setenv("CLAUDE_CONFIG_DIR", base)

	usages, err := claude.NewCollector().Collect(context.Background(), "syn-session")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.Equal(t, "claude-3-5-sonnet", usages[0].Model)
}

func TestClaudeCollector_Collect_FileNotFound(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	_, err := claude.NewCollector().Collect(context.Background(), "nonexistent-session")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ai.ErrSessionDataNotFound)
}
