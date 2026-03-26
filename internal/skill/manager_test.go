package skill_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/skill"
)

func newTestManager(t *testing.T) (*skill.Manager, string, string) {
	t.Helper()
	stateDir := t.TempDir()
	globalSkillsDir := t.TempDir()
	m := skill.NewManager(stateDir, globalSkillsDir)
	return m, stateDir, globalSkillsDir
}

func TestManager_CreateAndList(t *testing.T) {
	m, _, globalDir := newTestManager(t)

	err := m.Create("brainstorm", "Brainstorm discussions", "---\nname: brainstorm\n---\nThink!")
	require.NoError(t, err)

	// Global symlink should exist.
	link := filepath.Join(globalDir, "brainstorm")
	_, err = os.Lstat(link)
	require.NoError(t, err)

	skills, err := m.List()
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "brainstorm", skills[0].Name)
	assert.Equal(t, "v1", skills[0].ActiveVersion)
	assert.Equal(t, skill.SkillSourceManaged, skills[0].Source)
}

func TestManager_EditCreatesNewVersion(t *testing.T) {
	m, stateDir, _ := newTestManager(t)
	require.NoError(t, m.Create("commit", "Commit", "v1 content"))

	require.NoError(t, m.Edit("commit", "v2 content"))

	// v2 should exist in registry.
	v2Dir := filepath.Join(stateDir, "skills", "commit", "v2")
	assert.DirExists(t, v2Dir)

	// Global version should still be v1 (edit ≠ publish).
	cfg, err := m.LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "v1", cfg.Skills["commit"].GlobalVersion)
	assert.Equal(t, "v2", cfg.Skills["commit"].LatestVersion)
}

func TestManager_UseGlobal(t *testing.T) {
	m, _, globalDir := newTestManager(t)
	require.NoError(t, m.Create("deploy", "Deploy", "deploy v1"))
	require.NoError(t, m.Edit("deploy", "deploy v2"))

	require.NoError(t, m.UseGlobal("deploy", "v2"))

	target, err := os.Readlink(filepath.Join(globalDir, "deploy"))
	require.NoError(t, err)
	assert.Contains(t, target, "v2")

	cfg, err := m.LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "v2", cfg.Skills["deploy"].GlobalVersion)
}

func TestManager_UseWorker(t *testing.T) {
	m, _, _ := newTestManager(t)
	workDir := t.TempDir()

	require.NoError(t, m.Create("review", "Review", "review v1"))
	require.NoError(t, m.Edit("review", "review v2"))
	require.NoError(t, m.UseWorker("worker1", workDir, "review", "v1"))

	link := filepath.Join(workDir, ".claude", "skills", "review")
	target, err := os.Readlink(link)
	require.NoError(t, err)
	assert.Contains(t, target, "v1")
}

func TestManager_DeleteRefusedWithReferences(t *testing.T) {
	m, _, _ := newTestManager(t)
	workDir := t.TempDir()

	require.NoError(t, m.Create("shared", "Shared", "content"))
	require.NoError(t, m.UseWorker("worker1", workDir, "shared", "v1"))

	err := m.Delete("shared")
	assert.Error(t, err) // must refuse because worker1 references it
}

func TestManager_DeleteSucceeds(t *testing.T) {
	m, _, globalDir := newTestManager(t)
	require.NoError(t, m.Create("temp", "Temp", "content"))

	require.NoError(t, m.Delete("temp"))

	assert.NoFileExists(t, filepath.Join(globalDir, "temp"))
}

func TestManager_RemoveWorkerOverride(t *testing.T) {
	m, _, _ := newTestManager(t)
	workDir := t.TempDir()

	require.NoError(t, m.Create("myskill", "MySkill", "content"))
	require.NoError(t, m.UseWorker("worker1", workDir, "myskill", "v1"))

	// Override exists — symlink in worker dir.
	link := filepath.Join(workDir, ".claude", "skills", "myskill")
	_, err := os.Lstat(link)
	require.NoError(t, err)

	require.NoError(t, m.RemoveWorkerOverride("worker1", workDir, "myskill"))

	// Symlink should be gone.
	assert.NoFileExists(t, link)

	// Config should have no override for worker1.
	cfg, err := m.LoadConfig()
	require.NoError(t, err)
	_, hasOverride := cfg.WorkerOverrides["worker1"]
	assert.False(t, hasOverride)
}

func TestManager_CleanupWorkerLinks(t *testing.T) {
	m, _, _ := newTestManager(t)
	workDir := t.TempDir()

	require.NoError(t, m.Create("skill-a", "A", "content a"))
	require.NoError(t, m.Create("skill-b", "B", "content b"))
	require.NoError(t, m.UseWorker("worker1", workDir, "skill-a", "v1"))
	require.NoError(t, m.UseWorker("worker1", workDir, "skill-b", "v1"))

	require.NoError(t, m.CleanupWorkerLinks("worker1", workDir))

	// Both symlinks should be gone.
	assert.NoFileExists(t, filepath.Join(workDir, ".claude", "skills", "skill-a"))
	assert.NoFileExists(t, filepath.Join(workDir, ".claude", "skills", "skill-b"))

	// Worker overrides cleared in config.
	cfg, err := m.LoadConfig()
	require.NoError(t, err)
	_, hasOverrides := cfg.WorkerOverrides["worker1"]
	assert.False(t, hasOverrides)
}

func TestManager_CreateRejectsInvalidName(t *testing.T) {
	m, _, _ := newTestManager(t)

	err := m.Create("../evil", "Evil", "content")
	assert.Error(t, err)

	err = m.Create("skill/with/slash", "Bad", "content")
	assert.Error(t, err)

	err = m.Create("superpowers:brainstorm", "Valid with colon", "content")
	assert.NoError(t, err)

	err = m.Create("my-skill", "Valid with dash", "content")
	assert.NoError(t, err)
}

func TestManager_Adopt(t *testing.T) {
	m, stateDir, globalDir := newTestManager(t)

	// Simulate an externally-placed skill.
	extSkill := filepath.Join(globalDir, "legacy")
	require.NoError(t, os.MkdirAll(extSkill, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(extSkill, "SKILL.md"), []byte("legacy content"), 0o644))

	require.NoError(t, m.AdoptGlobal("legacy"))

	// Original path should now be a symlink.
	target, err := os.Readlink(filepath.Join(globalDir, "legacy"))
	require.NoError(t, err)
	assert.Contains(t, target, "v1")

	// Registry should have the content.
	v1Dir := filepath.Join(stateDir, "skills", "legacy", "v1")
	assert.DirExists(t, v1Dir)
}
