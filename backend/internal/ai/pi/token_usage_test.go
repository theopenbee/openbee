package pi_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/pi"
)

func writePiTempFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
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
	collector := pi.NewCollector()

	usages, err := collector.Collect(context.Background(), sessionID)
	require.NoError(t, err)
	byModel := map[string]ai.TokenUsage{}
	for _, u := range usages {
		byModel[u.Model] = u
	}

	sonnet := byModel["claude-3-5-sonnet"]
	assert.Equal(t, int64(300), sonnet.InputTokens)
	assert.Equal(t, int64(130), sonnet.OutputTokens)
	assert.Equal(t, int64(10), sonnet.CacheCreationTokens)
	assert.Equal(t, int64(20), sonnet.CacheReadTokens)

	opus := byModel["claude-3-opus"]
	assert.Equal(t, int64(300), opus.InputTokens)
}

func TestPiCollector_Collect_SkipsNonAssistantAndWrongType(t *testing.T) {
	sessionsDir := t.TempDir()
	sessionID := "skip-test"

	writePiTempFile(t, sessionsDir, sessionID+".jsonl", `{"type":"message","message":{"role":"user","model":"claude-3-5-sonnet","usage":{"input":999,"output":999}}}
{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":100,"output":50}}}
{"type":"other","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":999,"output":999}}}
`)
	t.Setenv("PI_AGENT_DIR", sessionsDir)

	usages, err := pi.NewCollector().Collect(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.Equal(t, int64(100), usages[0].InputTokens)
}

func TestPiCollector_Collect_FileNotFound(t *testing.T) {
	t.Setenv("PI_AGENT_DIR", t.TempDir())
	_, err := pi.NewCollector().Collect(context.Background(), "nonexistent-session")
	require.ErrorIs(t, err, ai.ErrSessionDataNotFound)
}

func TestPiCollector_Collect_DirectoryNotFound(t *testing.T) {
	t.Setenv("PI_AGENT_DIR", t.TempDir()+"/missing")
	_, err := pi.NewCollector().Collect(context.Background(), "nonexistent-session")
	require.ErrorIs(t, err, ai.ErrSessionDataNotFound)
}

func TestPiCollector_Collect_UsesOpenbeeDefaultSessionsDir(t *testing.T) {
	home := t.TempDir()
	sessionID := "default-dir-session"
	t.Setenv("HOME", home)
	t.Setenv("PI_AGENT_DIR", "")

	writePiTempFile(t, home, ".openbee/.pi/sessions/"+sessionID+".jsonl", `{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":100,"output":50}}}`)

	usages, err := pi.NewCollector().Collect(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, usages, 1)
	require.Equal(t, int64(100), usages[0].InputTokens)
	require.Equal(t, int64(50), usages[0].OutputTokens)
}
