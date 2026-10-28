package codex

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildArgs_NewSession(t *testing.T) {
	args := buildArgs("", false, "", nil)
	want := []string{"exec", "-", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	assert.Equal(t, want, args)
}

func TestBuildArgs_ResumeWithID(t *testing.T) {
	args := buildArgs("sess-123", true, "", nil)
	want := []string{"exec", "resume", "sess-123", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	assert.Equal(t, want, args)
}

func TestBuildArgs_ResumeWithIDAndPrompt(t *testing.T) {
	args := buildArgs("sess-123", true, "do something", nil)
	want := []string{"exec", "resume", "sess-123", "--json", "--dangerously-bypass-approvals-and-sandbox", "do something"}
	assert.Equal(t, want, args)
}

func TestExtractSessionID(t *testing.T) {
	jsonStream := `{"type":"thread.started","thread_id":"019d7293-0a51-71e0-b634-02183839d7b2"}
{"type":"turn.started"}
{"type":"turn.completed","usage":{}}
`
	id := extractSessionID(strings.NewReader(jsonStream))
	assert.Equal(t, "019d7293-0a51-71e0-b634-02183839d7b2", id)
}

func TestBuildArgs_ResumeUsesThreadID(t *testing.T) {
	args := buildArgs("thread-xyz-from-store", true, "follow up", nil)
	want := []string{"exec", "resume", "thread-xyz-from-store", "--json", "--dangerously-bypass-approvals-and-sandbox", "follow up"}
	assert.Equal(t, want, args)
}

func TestExtractResultFromLog(t *testing.T) {
	jsonStream := `{"type":"thread.started","thread_id":"abc"}
{"type":"item.completed","item":{"type":"agent_message","text":"hello world"}}
{"type":"turn.completed","usage":{}}
`
	tmpFile := t.TempDir() + "/test.log"
	require.NoError(t, os.WriteFile(tmpFile, []byte(jsonStream), 0o644))
	result := ExtractResultFromLog(tmpFile)
	assert.Equal(t, "hello world", result)
}
