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

func TestBeeSkillKeepsWorkerManagementForBee(t *testing.T) {
	data, err := skillsFS.ReadFile("skills/openbee-bee/SKILL.md")
	require.NoError(t, err)
	skill := string(data)

	assert.False(t, strings.Contains(skill, "explicitly mentions the name of an **existing** worker"), "Rule 1 still dispatches on any mentioned name")
	for _, rule := range []string{
		"**Management operations are always yours.**",
		"A name that is only the subject or object of the request",
		"Removing that name from data, code, files or pages is not deleting the worker",
		"never turn it into a context reset",
		"Never call `delete_worker` on the message that requests the deletion",
		"View existing task list and task details, cancel tasks",
	} {
		assert.Truef(t, strings.Contains(skill, rule), "missing rule %q", rule)
	}
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
