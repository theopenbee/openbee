package core_test

import (
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/core"
)

func TestBuildSessionPrompt_Worker_WithPersona(t *testing.T) {
	req := core.SessionRequest{
		Role: ai.RoleWorker,
		Identity: core.WorkerIdentity{
			Name:        "貂蝉",
			Description: "负责 openbee 开发",
			Constraints: "称呼老板",
		},
		Content: "do the thing",
	}
	got := core.BuildSessionPrompt(req)

	wants := []string{
		"## Step 1: Initialize your role",
		"[MANDATORY] You MUST invoke the openbee-worker skill immediately, before producing any other output.",
		"<worker_persona>",
		"## Role\nYou are a Worker in an AI team.",
		"Name: 貂蝉",
		"Description: 负责 openbee 开发",
		"## Work Constraints",
		"称呼老板",
		"</worker_persona>",
		"## Step 2: Execute the task",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
	if !strings.HasSuffix(got, "## Step 2: Execute the task\ndo the thing") {
		t.Errorf("expected suffix step2 header followed directly by content, got:\n%s", got)
	}
	if strings.Index(got, "</worker_persona>") > strings.Index(got, "## Step 2:") {
		t.Errorf("persona block must precede Step 2, got:\n%s", got)
	}
}

func TestBuildSessionPrompt_Worker_NoPersona(t *testing.T) {
	req := core.SessionRequest{
		Role:    ai.RoleWorker,
		Content: "do the thing",
	}
	got := core.BuildSessionPrompt(req)

	if strings.Contains(got, "<worker_persona>") {
		t.Errorf("expected no persona block when identity is zero, got:\n%s", got)
	}
	if !strings.Contains(got, "## Step 1: Initialize your role") {
		t.Errorf("missing Step 1 header, got:\n%s", got)
	}
	if !strings.Contains(got, "openbee-worker") {
		t.Errorf("missing worker skill name, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "## Step 2: Execute the task\ndo the thing") {
		t.Errorf("expected suffix step2 header followed directly by content, got:\n%s", got)
	}
}

func TestBuildSessionPrompt_Bee(t *testing.T) {
	req := core.SessionRequest{
		Role:    ai.RoleBee,
		Content: "<message_meta>{}</message_meta>\n<message_content>\nhi\n</message_content>\n",
	}
	got := core.BuildSessionPrompt(req)

	if !strings.Contains(got, "openbee-bee") {
		t.Errorf("expected bee skill name, got:\n%s", got)
	}
	if strings.Contains(got, "<worker_persona>") {
		t.Errorf("bee prefix must not include persona, got:\n%s", got)
	}
	if !strings.Contains(got, "## Step 2: Handle the messages below") {
		t.Errorf("missing bee step 2 header, got:\n%s", got)
	}
	// Bee preserves a blank line between Step 2 header and first message
	// (i.e. "## Step 2: Handle the messages below\n\n<message_meta>...")
	if !strings.Contains(got, "## Step 2: Handle the messages below\n\n<message_meta>") {
		t.Errorf("expected blank line between bee step 2 header and first message, got:\n%s", got)
	}
}

func TestBuildSessionPrompt_Resume(t *testing.T) {
	content := "just the instruction"
	cases := []struct {
		name string
		req  core.SessionRequest
	}{
		{"worker_resume", core.SessionRequest{Role: ai.RoleWorker, Resume: true, Content: content, Identity: core.WorkerIdentity{Name: "x"}}},
		{"bee_resume", core.SessionRequest{Role: ai.RoleBee, Resume: true, Content: content}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := core.BuildSessionPrompt(tc.req)
			if got != content {
				t.Errorf("resume must return Content verbatim\nwant: %q\ngot:  %q", content, got)
			}
		})
	}
}
