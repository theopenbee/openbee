package ai

import (
	"strings"
	"testing"
)

func TestSkillHintPrefix_Bee(t *testing.T) {
	got := SkillHintPrefix(RoleBee)
	want := "[MANDATORY] You MUST invoke the openbee-bee skill immediately. This is your FIRST and ONLY action before doing anything else. Do NOT skip this step. Do NOT produce any text output before invoking the skill."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSkillHintPrefix_Worker(t *testing.T) {
	got := SkillHintPrefix(RoleWorker)
	want := "[MANDATORY] You MUST invoke the openbee-worker skill immediately. This is your FIRST and ONLY action before doing anything else. Do NOT skip this step. Do NOT produce any text output before invoking the skill."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSkillHintPrefix_Unknown(t *testing.T) {
	got := SkillHintPrefix(Role("other"))
	if got != "" {
		t.Errorf("expected empty string for unknown role, got %q", got)
	}
}

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
