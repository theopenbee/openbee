package codex_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/codex"
)

func TestAdapter_Prepare_NoOp(t *testing.T) {
	dir := t.TempDir()
	a, err := codex.NewAdapter("echo", nil)
	require.NoError(t, err)

	require.NoError(t, a.Prepare(dir, ai.PrepareOptions{Role: ai.RoleBee}))
	// Prepare must not create any files
	entries, _ := os.ReadDir(dir)
	assert.Empty(t, entries, "Prepare must not create files")
	_ = filepath.Join(dir, "AGENTS.md") // Ensure path helpers compile
}

func TestAdapter_Prepare_BothRoles(t *testing.T) {
	a, err := codex.NewAdapter("echo", nil)
	require.NoError(t, err)
	for _, role := range []ai.Role{ai.RoleBee, ai.RoleWorker} {
		dir := t.TempDir()
		err := a.Prepare(dir, ai.PrepareOptions{Role: role})
		assert.NoError(t, err, "Prepare(%s)", role)
	}
}

func TestAdapter_ExtraEnvInBaseEnv(t *testing.T) {
	a, err := codex.NewAdapter("echo", map[string]string{
		"CODEX_CUSTOM": "value",
	})
	require.NoError(t, err)
	var _ ai.EngineAdapter = a
}
