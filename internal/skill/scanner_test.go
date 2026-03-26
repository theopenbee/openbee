package skill_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/skill"
)

func TestScanner_ManagedGlobal(t *testing.T) {
	registryDir := t.TempDir()
	globalSkillsDir := t.TempDir()

	// Create registry version.
	vDir := filepath.Join(registryDir, "brainstorm", "v1")
	require.NoError(t, os.MkdirAll(vDir, 0o755))

	// Create managed symlink.
	lm := skill.NewLinkManager(registryDir, globalSkillsDir)
	require.NoError(t, lm.SetGlobal("brainstorm", "v1"))

	sc := skill.NewScanner(registryDir, globalSkillsDir)
	results, err := sc.ScanGlobal()
	require.NoError(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, "brainstorm", results[0].Name)
	assert.Equal(t, skill.SkillSourceManaged, results[0].Source)
	assert.Equal(t, "v1", results[0].ActiveVersion)
}

func TestScanner_ExternalGlobal(t *testing.T) {
	registryDir := t.TempDir()
	globalSkillsDir := t.TempDir()

	// Place a real (non-symlink) skill directory.
	extSkill := filepath.Join(globalSkillsDir, "external-skill")
	require.NoError(t, os.MkdirAll(extSkill, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(extSkill, "SKILL.md"), []byte("---\n---\nhello"), 0o644))

	sc := skill.NewScanner(registryDir, globalSkillsDir)
	results, err := sc.ScanGlobal()
	require.NoError(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, "external-skill", results[0].Name)
	assert.Equal(t, skill.SkillSourceExternal, results[0].Source)
	assert.Empty(t, results[0].ActiveVersion)
}

func TestScanner_WorkerSkill(t *testing.T) {
	registryDir := t.TempDir()
	globalSkillsDir := t.TempDir()
	workDir := t.TempDir()

	vDir := filepath.Join(registryDir, "commit", "v2")
	require.NoError(t, os.MkdirAll(vDir, 0o755))

	lm := skill.NewLinkManager(registryDir, globalSkillsDir)
	require.NoError(t, lm.SetWorker(workDir, "commit", "v2"))

	sc := skill.NewScanner(registryDir, globalSkillsDir)
	results, err := sc.ScanWorker("worker1", workDir)
	require.NoError(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, "commit", results[0].Name)
	assert.Equal(t, skill.SkillSourceManaged, results[0].Source)
	assert.Equal(t, "v2", results[0].ActiveVersion)
	assert.Equal(t, "worker1", results[0].Scope)
}
