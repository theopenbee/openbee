package ai

import (
	"strings"
	"testing"
)

func TestWorkerPersona_Full(t *testing.T) {
	got := WorkerPersona("mybot", "does things", "remember X")
	if !strings.Contains(got, "## Role\n") {
		t.Errorf("missing ## Role header, got: %q", got)
	}
	if !strings.Contains(got, "You are a Worker in an AI team.") {
		t.Errorf("missing persona line, got: %q", got)
	}
	if !strings.Contains(got, "## Identity\n") {
		t.Errorf("missing ## Identity header, got: %q", got)
	}
	if !strings.Contains(got, "Name: mybot") {
		t.Errorf("missing name, got: %q", got)
	}
	if !strings.Contains(got, "Description: does things") {
		t.Errorf("missing description, got: %q", got)
	}
	if !strings.Contains(got, "## Work Constraints") {
		t.Errorf("missing constraints header, got: %q", got)
	}
	if !strings.Contains(got, "remember X") {
		t.Errorf("missing constraints content, got: %q", got)
	}
	if strings.Contains(got, "openbee-worker") {
		t.Errorf("persona must NOT contain skill rule directive, got: %q", got)
	}
}

func TestWorkerPersona_Empty(t *testing.T) {
	got := WorkerPersona("", "", "")
	if got != "## Role\nYou are a Worker in an AI team.\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuildSessionPrefix_WorkerWithPersona(t *testing.T) {
	persona := WorkerPersona("貂蝉", "负责 openbee 开发", "称呼老板")
	got := BuildSessionPrefix(RoleWorker, persona)

	wants := []string{
		"## Step 1: Initialize your role",
		"[MANDATORY] You MUST invoke the openbee-worker skill immediately, before producing any other output.",
		"<worker_persona>",
		"Name: 貂蝉",
		"Description: 负责 openbee 开发",
		"称呼老板",
		"</worker_persona>",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
	if !strings.HasSuffix(got, "## Step 2: Execute the task\n") {
		t.Errorf("expected suffix %q, got:\n%s", "## Step 2: Execute the task\n", got)
	}
	if strings.Index(got, "</worker_persona>") > strings.Index(got, "## Step 2:") {
		t.Errorf("persona block must precede Step 2, got:\n%s", got)
	}
}

func TestBuildSessionPrefix_WorkerNoPersona(t *testing.T) {
	got := BuildSessionPrefix(RoleWorker, "")

	if strings.Contains(got, "<worker_persona>") {
		t.Errorf("expected no persona block when persona is empty, got:\n%s", got)
	}
	if !strings.Contains(got, "## Step 1: Initialize your role") {
		t.Errorf("missing Step 1 header, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "## Step 2: Execute the task\n") {
		t.Errorf("expected suffix %q, got:\n%s", "## Step 2: Execute the task\n", got)
	}
}

func TestBuildSessionPrefix_Bee(t *testing.T) {
	got := BuildSessionPrefix(RoleBee, "")

	if !strings.Contains(got, "openbee-bee") {
		t.Errorf("expected bee skill name, got:\n%s", got)
	}
	if strings.Contains(got, "<worker_persona>") {
		t.Errorf("bee prefix must not include persona, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "## Step 2: Handle the messages below\n") {
		t.Errorf("expected suffix %q, got:\n%s", "## Step 2: Handle the messages below\n", got)
	}
}

func TestBuildSessionPrefix_BeeIgnoresPersona(t *testing.T) {
	got := BuildSessionPrefix(RoleBee, WorkerPersona("ghost", "should be ignored", ""))
	if strings.Contains(got, "<worker_persona>") {
		t.Errorf("Bee prefix must not include persona even when one is passed, got:\n%s", got)
	}
	if strings.Contains(got, "ghost") {
		t.Errorf("Bee prefix must not leak persona content, got:\n%s", got)
	}
}

func TestBuildSessionPrefix_UnknownRole(t *testing.T) {
	if got := BuildSessionPrefix(Role("other"), ""); got != "" {
		t.Errorf("expected empty string for unknown role, got %q", got)
	}
}
