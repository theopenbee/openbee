package skill_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/skill"
)

func TestRegistry_CreateAndRead(t *testing.T) {
	dir := t.TempDir()
	r := skill.NewRegistry(dir)

	content := "---\nname: test\n---\nHello"
	versionDir, err := r.CreateVersion("myskill", content)
	require.NoError(t, err)
	assert.DirExists(t, versionDir)

	data, err := os.ReadFile(filepath.Join(versionDir, "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestRegistry_VersionSequence(t *testing.T) {
	dir := t.TempDir()
	r := skill.NewRegistry(dir)

	v1, err := r.CreateVersion("seq", "content v1")
	require.NoError(t, err)
	assert.Contains(t, v1, "v1")

	v2, err := r.CreateVersion("seq", "content v2")
	require.NoError(t, err)
	assert.Contains(t, v2, "v2")
}

func TestRegistry_ReadVersion(t *testing.T) {
	dir := t.TempDir()
	r := skill.NewRegistry(dir)

	_, err := r.CreateVersion("skill1", "initial content")
	require.NoError(t, err)

	content, err := r.ReadVersion("skill1", "v1")
	require.NoError(t, err)
	assert.Equal(t, "initial content", content)
}

func TestRegistry_VersionPath(t *testing.T) {
	dir := t.TempDir()
	r := skill.NewRegistry(dir)

	path := r.VersionPath("myskill", "v2")
	assert.Equal(t, filepath.Join(dir, "myskill", "v2"), path)
}

func TestRegistry_DeleteVersion_RefusedWhenLast(t *testing.T) {
	dir := t.TempDir()
	r := skill.NewRegistry(dir)

	_, err := r.CreateVersion("solo", "only version")
	require.NoError(t, err)

	err = r.DeleteVersion("solo", "v1")
	assert.ErrorIs(t, err, skill.ErrLastVersion)
}

func TestRegistry_DeleteSkill(t *testing.T) {
	dir := t.TempDir()
	r := skill.NewRegistry(dir)

	_, err := r.CreateVersion("todelete", "content")
	require.NoError(t, err)

	require.NoError(t, r.DeleteSkill("todelete"))
	assert.NoDirExists(t, filepath.Join(dir, "todelete"))
}
