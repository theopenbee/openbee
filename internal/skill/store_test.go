package skill_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/skill"
)

func TestStore_LoadEmpty(t *testing.T) {
	dir := t.TempDir()
	s := skill.NewStore(filepath.Join(dir, "skills.json"))
	cfg, err := s.Load()
	require.NoError(t, err)
	assert.Equal(t, 1, cfg.Version)
	assert.NotNil(t, cfg.Skills)
	assert.NotNil(t, cfg.WorkerOverrides)
}

func TestStore_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	s := skill.NewStore(filepath.Join(dir, "skills.json"))

	cfg := skill.SkillsConfig{
		Version: 1,
		Skills: map[string]skill.SkillEntry{
			"brainstorming": {
				Description:   "Brainstorm",
				LatestVersion: "v1",
				GlobalVersion: "v1",
				Versions: map[string]skill.VersionEntry{
					"v1": {CreatedAt: time.Now().UTC().Truncate(time.Second)},
				},
			},
		},
		WorkerOverrides: map[string]map[string]string{},
	}

	require.NoError(t, s.Save(cfg))

	loaded, err := s.Load()
	require.NoError(t, err)
	assert.Equal(t, "Brainstorm", loaded.Skills["brainstorming"].Description)
	assert.Equal(t, "v1", loaded.Skills["brainstorming"].GlobalVersion)
}
