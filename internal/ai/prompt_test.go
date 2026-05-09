package ai

import (
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/model"
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

func TestBuildSystemPrompt_Bee(t *testing.T) {
	got := BuildSystemPrompt(RoleBee, nil)
	if !strings.HasPrefix(got, SkillHintPrefix(RoleBee)) {
		t.Errorf("bee system prompt must start with skill hint, got: %q", got)
	}
	if strings.Contains(got, "<worker_persona>") {
		t.Errorf("bee system prompt must not include worker_persona, got: %q", got)
	}
}

func TestBuildSystemPrompt_Worker_WithPersona(t *testing.T) {
	w := &model.Worker{Name: "貂蝉", Description: "负责 openbee 开发", Constraints: "称呼用户老板"}
	got := BuildSystemPrompt(RoleWorker, w)
	if !strings.HasPrefix(got, SkillHintPrefix(RoleWorker)) {
		t.Errorf("worker system prompt must start with skill hint, got: %q", got)
	}
	if !strings.Contains(got, "<worker_persona>") || !strings.Contains(got, "</worker_persona>") {
		t.Errorf("worker system prompt must wrap persona in <worker_persona> tags, got: %q", got)
	}
	for _, want := range []string{"Name: 貂蝉", "Description: 负责 openbee 开发", "称呼用户老板"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in system prompt, got: %q", want, got)
		}
	}
}

func TestBuildSystemPrompt_Worker_NilWorker(t *testing.T) {
	got := BuildSystemPrompt(RoleWorker, nil)
	if got != SkillHintPrefix(RoleWorker) {
		t.Errorf("nil worker should yield only the skill hint, got: %q", got)
	}
}

func TestBuildSystemPrompt_UnknownRole(t *testing.T) {
	got := BuildSystemPrompt(Role("other"), nil)
	if got != "" {
		t.Errorf("unknown role must return empty string, got: %q", got)
	}
}

func TestPrependSystemPrompt_Empty(t *testing.T) {
	got := PrependSystemPrompt("hello", "")
	if got != "hello" {
		t.Errorf("empty system prompt must not modify user prompt, got: %q", got)
	}
}

func TestPrependSystemPrompt_Prepends(t *testing.T) {
	got := PrependSystemPrompt("hello", "be terse")
	want := "be terse\n\nhello"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
