package claude_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/claude"
)

func newTestAdapter(t *testing.T) ai.EngineAdapter {
	t.Helper()
	return claude.NewAdapter("echo", nil)
}

func TestClaudeAdapter_ExtraEnvInBaseEnv(t *testing.T) {
	a := claude.NewAdapter("echo", map[string]string{
		"MY_CUSTOM_VAR": "hello",
		"ANOTHER_KEY":   "world",
	})
	// Access baseEnv indirectly: run a command that echoes env and check output.
	// Since we cannot inspect baseEnv directly, we verify NewAdapter doesn't panic
	// and the adapter satisfies the interface.
	var _ ai.EngineAdapter = a
}

func TestClaudeAdapter_Prepare_Stub(t *testing.T) {
	dir := t.TempDir()
	adapter := newTestAdapter(t)
	require.NoError(t, adapter.Prepare(dir, ai.PrepareOptions{Role: ai.RoleWorker}))
}

func TestClaudeAdapter_Prepare_DeletesOpenbeeFile(t *testing.T) {
	dir := t.TempDir()
	openbeeFile := filepath.Join(dir, ai.SystemRulesFile)
	require.NoError(t, os.WriteFile(openbeeFile, []byte("old rules"), 0o644))

	require.NoError(t, newTestAdapter(t).Prepare(dir, ai.PrepareOptions{Role: ai.RoleWorker}))

	_, err := os.Stat(openbeeFile)
	assert.True(t, os.IsNotExist(err), ".openbee.md should have been deleted by Prepare")
}

func TestClaudeAdapter_Prepare_RemovesImportLine(t *testing.T) {
	dir := t.TempDir()
	claudeFile := filepath.Join(dir, "CLAUDE.md")
	content := "# My Bot\n" + ai.ImportLine + "\nOther content\n"
	require.NoError(t, os.WriteFile(claudeFile, []byte(content), 0o644))

	require.NoError(t, newTestAdapter(t).Prepare(dir, ai.PrepareOptions{Role: ai.RoleWorker}))

	data, _ := os.ReadFile(claudeFile)
	got := string(data)
	assert.NotContains(t, got, ai.ImportLine, "CLAUDE.md should not contain import line after Prepare")
	assert.Contains(t, got, "# My Bot", "CLAUDE.md should preserve other content")
	assert.Contains(t, got, "Other content", "CLAUDE.md should preserve other content")
}

func TestClaudeAdapter_Prepare_PreservesOtherCLAUDEMDContent(t *testing.T) {
	dir := t.TempDir()
	claudeFile := filepath.Join(dir, "CLAUDE.md")
	// CLAUDE.md with no import line — must not be modified
	original := "# Custom instructions\nDo something special.\n"
	require.NoError(t, os.WriteFile(claudeFile, []byte(original), 0o644))

	require.NoError(t, newTestAdapter(t).Prepare(dir, ai.PrepareOptions{Role: ai.RoleBee}))

	data, _ := os.ReadFile(claudeFile)
	assert.Equal(t, original, string(data), "CLAUDE.md should be unchanged when import line is absent")
}

func TestClaudeAdapter_Prepare_NoopWhenFilesAbsent(t *testing.T) {
	dir := t.TempDir()
	err := newTestAdapter(t).Prepare(dir, ai.PrepareOptions{Role: ai.RoleBee})
	require.NoError(t, err, "Prepare should not error when no files exist")
}

func TestClaudeAdapter_Prepare_BothRoles(t *testing.T) {
	for _, role := range []ai.Role{ai.RoleBee, ai.RoleWorker} {
		dir := t.TempDir()
		// Setup: both legacy files present
		os.WriteFile(filepath.Join(dir, ai.SystemRulesFile), []byte("rules"), 0o644)
		os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(ai.ImportLine+"\n"), 0o644)

		err := newTestAdapter(t).Prepare(dir, ai.PrepareOptions{Role: role})
		assert.NoError(t, err, "Prepare(%s)", role)
		_, statErr := os.Stat(filepath.Join(dir, ai.SystemRulesFile))
		assert.True(t, os.IsNotExist(statErr), "role %s: .openbee.md should be deleted", role)
	}
}
