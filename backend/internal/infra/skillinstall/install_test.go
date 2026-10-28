package skillinstall

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallSkills_UpToDate(t *testing.T) {
	dir := t.TempDir()
	_, err := InstallSkills(dir)
	require.NoError(t, err, "first install failed")
	results, err := InstallSkills(dir)
	require.NoError(t, err, "second install failed")
	for _, r := range results {
		assert.Equal(t, ActionUpToDate, r.Action, "skill %s", r.Name)
	}
}

func TestInstallSkills_UpdatedWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := InstallSkills(dir)
	require.NoError(t, err, "first install failed")
	// Delete a reference file from openbee-bee.
	missing := filepath.Join(dir, "openbee-bee", "references", "cli-reference.md")
	require.NoError(t, os.Remove(missing))
	results, err := InstallSkills(dir)
	require.NoError(t, err, "reinstall failed")
	var beeResult SkillResult
	for _, r := range results {
		if r.Name == "openbee-bee" {
			beeResult = r
		}
	}
	assert.Equal(t, ActionUpdated, beeResult.Action, "openbee-bee when file missing")
	_, err = os.Stat(missing)
	assert.NoError(t, err, "missing file was not restored")
}

func TestInstallSkills_PrunesStaleFile(t *testing.T) {
	dir := t.TempDir()
	_, err := InstallSkills(dir)
	require.NoError(t, err, "first install failed")
	// Plant a stale file that is not in the embedded FS.
	stale := filepath.Join(dir, "openbee-worker", "references", "old-removed.md")
	require.NoError(t, os.WriteFile(stale, []byte("stale content"), 0o644))
	results, err := InstallSkills(dir)
	require.NoError(t, err, "reinstall failed")
	var workerResult SkillResult
	for _, r := range results {
		if r.Name == "openbee-worker" {
			workerResult = r
		}
	}
	assert.Equal(t, ActionUpdated, workerResult.Action, "openbee-worker when stale file present")
	_, err = os.Stat(stale)
	assert.Error(t, err, "stale file was not pruned")
}

func TestInstallSkills_Updated(t *testing.T) {
	dir := t.TempDir()
	// Write stale SKILL.md for openbee-bee.
	beeDir := filepath.Join(dir, "openbee-bee")
	require.NoError(t, os.MkdirAll(beeDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(beeDir, "SKILL.md"), []byte("stale content"), 0o644))
	results, err := InstallSkills(dir)
	require.NoError(t, err)
	var beeResult SkillResult
	for _, r := range results {
		if r.Name == "openbee-bee" {
			beeResult = r
		}
	}
	assert.Equal(t, ActionUpdated, beeResult.Action, "openbee-bee")
	// Verify SKILL.md now matches embedded content.
	embedded, _ := collectEmbeddedFiles("skills/openbee-bee")
	got, _ := os.ReadFile(filepath.Join(beeDir, "SKILL.md"))
	assert.Equal(t, embedded["SKILL.md"], string(got), "openbee-bee SKILL.md content not updated to embedded content")
	var workerResult SkillResult
	for _, r := range results {
		if r.Name == "openbee-worker" {
			workerResult = r
		}
	}
	assert.Equal(t, ActionInstalled, workerResult.Action, "openbee-worker")
}

func TestInstallSkillsToDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	results, err := InstallSkillsToDefaults()
	require.NoError(t, err)
	// Two dirs × number of skills = total results.
	want := 2 * len(embeddedSkills)
	require.Len(t, results, want)
	for _, r := range results {
		assert.Equal(t, ActionInstalled, r.Action, "skill %s", r.Name)
	}

	// Verify all embedded files exist, with matching content, in both target directories.
	claudeSkills := filepath.Join(home, ".claude", "skills")
	agentsSkills := filepath.Join(home, ".agents", "skills")
	for _, dir := range []string{claudeSkills, agentsSkills} {
		for _, name := range embeddedSkills {
			embedded, err := collectEmbeddedFiles("skills/" + name)
			require.NoError(t, err, "skill %s: collect embedded files", name)
			for relPath, wantContent := range embedded {
				p := filepath.Join(dir, name, filepath.FromSlash(relPath))
				got, err := os.ReadFile(p)
				if !assert.NoError(t, err, "expected %s to exist", p) {
					continue
				}
				assert.Equal(t, wantContent, string(got), "skill %s: file %s content mismatch", name, relPath)
			}
		}
	}
}

func TestInstallSkills_WriteError(t *testing.T) {
	dir := t.TempDir()
	// Block dir creation for openbee-bee by placing a file where the dir would go.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "openbee-bee"), []byte("block"), 0o644))
	_, err := InstallSkills(dir)
	require.Error(t, err)
}
