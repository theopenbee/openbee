package skillinstall

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func beeNotificationRule(t *testing.T, prefix string) string {
	t.Helper()
	data, err := skillsFS.ReadFile("skills/openbee-bee/SKILL.md")
	require.NoError(t, err)
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("no notification rule starts with %q", prefix)
	return ""
}

func TestBeeSkillLocalDispatchSendsNoText(t *testing.T) {
	receipt := beeNotificationRule(t, "1. **When a user request is received**")
	assert.Contains(t, receipt, `from is "local"`)
	assert.Contains(t, receipt, "handoff card")

	dispatched := beeNotificationRule(t, "2. **When a task is dispatched**")
	assert.Contains(t, dispatched, `from is "local"`)
	assert.Contains(t, dispatched, "inside another message")
}

func TestSkillsDoNotMentionRemovedCLI(t *testing.T) {
	err := fs.WalkDir(skillsFS, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := skillsFS.ReadFile(path)
		require.NoError(t, err)
		assert.NotContains(t, string(data), "openbee ctl", path)
		return nil
	})
	require.NoError(t, err)
}
