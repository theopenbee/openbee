package ai

import (
	"strings"
	"testing"
)

func TestSkillHintPrefix_Bee(t *testing.T) {
	got := SkillHintPrefix(RoleBee)
	if got != "use openbee-bee skill." {
		t.Errorf("got %q, want %q", got, "use openbee-bee skill.")
	}
}

func TestSkillHintPrefix_Worker(t *testing.T) {
	got := SkillHintPrefix(RoleWorker)
	if got != "use openbee-worker skill." {
		t.Errorf("got %q, want %q", got, "use openbee-worker skill.")
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
	if !strings.Contains(got, "You are a Worker in an AI team.") {
		t.Errorf("missing persona line, got: %q", got)
	}
	if !strings.Contains(got, "Name: mybot") {
		t.Errorf("missing name, got: %q", got)
	}
	if !strings.Contains(got, "Description: does things") {
		t.Errorf("missing description, got: %q", got)
	}
	if !strings.Contains(got, "## Memory Constraints") {
		t.Errorf("missing memory header, got: %q", got)
	}
	if !strings.Contains(got, "remember X") {
		t.Errorf("missing memory content, got: %q", got)
	}
	if strings.Contains(got, "openbee-worker") {
		t.Errorf("persona must NOT contain skill rule directive, got: %q", got)
	}
}

func TestWorkerPersona_Empty(t *testing.T) {
	got := WorkerPersona("", "", "")
	if got != "You are a Worker in an AI team.\n" {
		t.Errorf("got %q", got)
	}
}
