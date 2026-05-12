package codex

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestBuildArgs_NewSession(t *testing.T) {
	args := buildArgs("", false, "", nil)
	want := []string{"exec", "-", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildArgs_ResumeWithID(t *testing.T) {
	args := buildArgs("sess-123", true, "", nil)
	want := []string{"exec", "resume", "sess-123", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildArgs_ResumeWithIDAndPrompt(t *testing.T) {
	args := buildArgs("sess-123", true, "do something", nil)
	want := []string{"exec", "resume", "sess-123", "--json", "--dangerously-bypass-approvals-and-sandbox", "do something"}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestExtractSessionID(t *testing.T) {
	jsonStream := `{"type":"thread.started","thread_id":"019d7293-0a51-71e0-b634-02183839d7b2"}
{"type":"turn.started"}
{"type":"turn.completed","usage":{}}
`
	id := extractSessionID(strings.NewReader(jsonStream))
	if id != "019d7293-0a51-71e0-b634-02183839d7b2" {
		t.Errorf("got %q, want 019d7293-0a51-71e0-b634-02183839d7b2", id)
	}
}

func TestBuildArgs_ResumeUsesThreadID(t *testing.T) {
	args := buildArgs("thread-xyz-from-store", true, "follow up", nil)
	want := []string{"exec", "resume", "thread-xyz-from-store", "--json", "--dangerously-bypass-approvals-and-sandbox", "follow up"}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestExtractResultFromLog(t *testing.T) {
	jsonStream := `{"type":"thread.started","thread_id":"abc"}
{"type":"item.completed","item":{"type":"agent_message","text":"hello world"}}
{"type":"turn.completed","usage":{}}
`
	tmpFile := t.TempDir() + "/test.log"
	if err := os.WriteFile(tmpFile, []byte(jsonStream), 0o644); err != nil {
		t.Fatal(err)
	}
	result := (&Backend{}).Extract(tmpFile)
	if result != "hello world" {
		t.Errorf("got %q, want %q", result, "hello world")
	}
}
