package skill_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/skill"
)

func TestLinks_SetGlobal(t *testing.T) {
	registryDir := t.TempDir()
	globalSkillsDir := t.TempDir()

	// Create a fake version directory in registry.
	versionDir := filepath.Join(registryDir, "brainstorming", "v1")
	require.NoError(t, os.MkdirAll(versionDir, 0o755))

	lm := skill.NewLinkManager(registryDir, globalSkillsDir)
	require.NoError(t, lm.SetGlobal("brainstorming", "v1"))

	link := filepath.Join(globalSkillsDir, "brainstorming")
	target, err := os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, versionDir, target)
}

func TestLinks_SetWorker(t *testing.T) {
	registryDir := t.TempDir()
	workDir := t.TempDir()
	globalSkillsDir := t.TempDir()

	versionDir := filepath.Join(registryDir, "commit", "v2")
	require.NoError(t, os.MkdirAll(versionDir, 0o755))

	lm := skill.NewLinkManager(registryDir, globalSkillsDir)
	require.NoError(t, lm.SetWorker(workDir, "commit", "v2"))

	link := filepath.Join(workDir, ".claude", "skills", "commit")
	target, err := os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, versionDir, target)
}

func TestLinks_RemoveGlobal(t *testing.T) {
	registryDir := t.TempDir()
	globalSkillsDir := t.TempDir()

	versionDir := filepath.Join(registryDir, "myskill", "v1")
	require.NoError(t, os.MkdirAll(versionDir, 0o755))

	lm := skill.NewLinkManager(registryDir, globalSkillsDir)
	require.NoError(t, lm.SetGlobal("myskill", "v1"))
	require.NoError(t, lm.RemoveGlobal("myskill"))

	assert.NoFileExists(t, filepath.Join(globalSkillsDir, "myskill"))
}

func TestLinks_RemoveWorker(t *testing.T) {
	registryDir := t.TempDir()
	workDir := t.TempDir()
	globalSkillsDir := t.TempDir()

	versionDir := filepath.Join(registryDir, "myskill", "v1")
	require.NoError(t, os.MkdirAll(versionDir, 0o755))

	lm := skill.NewLinkManager(registryDir, globalSkillsDir)
	require.NoError(t, lm.SetWorker(workDir, "myskill", "v1"))
	require.NoError(t, lm.RemoveWorker(workDir, "myskill"))

	assert.NoFileExists(t, filepath.Join(workDir, ".claude", "skills", "myskill"))
}

func TestLinks_RemoveAllWorkerLinks(t *testing.T) {
	registryDir := t.TempDir()
	workDir := t.TempDir()
	globalSkillsDir := t.TempDir()

	// Create a managed skill in registry and set worker link.
	versionDir := filepath.Join(registryDir, "managed-skill", "v1")
	require.NoError(t, os.MkdirAll(versionDir, 0o755))

	lm := skill.NewLinkManager(registryDir, globalSkillsDir)
	require.NoError(t, lm.SetWorker(workDir, "managed-skill", "v1"))

	// Also place an external (non-managed) real directory in worker skills.
	extSkillDir := filepath.Join(workDir, ".claude", "skills", "external-skill")
	require.NoError(t, os.MkdirAll(extSkillDir, 0o755))

	require.NoError(t, lm.RemoveAllWorkerLinks(workDir))

	// Managed symlink should be gone.
	assert.NoFileExists(t, filepath.Join(workDir, ".claude", "skills", "managed-skill"))
	// External real directory should remain.
	assert.DirExists(t, extSkillDir)
}
