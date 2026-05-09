package ai

import (
	"fmt"

	"github.com/theopenbee/openbee/internal/infra/model"
)

// WorkerPersona returns the persona-only content injected into new worker session prompts.
func WorkerPersona(name, description, constraints string) string {
	s := "## Role\nYou are a Worker in an AI team.\n"
	if name != "" || description != "" {
		s += "\n## Identity\n"
	}
	if name != "" {
		s += fmt.Sprintf("Name: %s\n", name)
	}
	if description != "" {
		s += fmt.Sprintf("Description: %s\n", description)
	}
	if constraints != "" {
		s += fmt.Sprintf("\n## Work Constraints\n%s\n", constraints)
	}
	return s
}

// SkillHintPrefix returns the skill invocation hint prepended to the first
// message of a new session.
func SkillHintPrefix(role Role) string {
	switch role {
	case RoleBee:
		return "[MANDATORY] You MUST invoke the openbee-bee skill immediately. This is your FIRST and ONLY action before doing anything else. Do NOT skip this step. Do NOT produce any text output before invoking the skill."
	case RoleWorker:
		return "[MANDATORY] You MUST invoke the openbee-worker skill immediately. This is your FIRST and ONLY action before doing anything else. Do NOT skip this step. Do NOT produce any text output before invoking the skill."
	default:
		return ""
	}
}

// BuildSystemPrompt returns the session-level system instructions for the
// given role, or "" for unknown roles.
func BuildSystemPrompt(role Role, w *model.Worker) string {
	hint := SkillHintPrefix(role)
	if hint == "" {
		return ""
	}
	if role == RoleWorker && w != nil {
		persona := WorkerPersona(w.Name, w.Description, w.Constraints)
		return hint + "\n<worker_persona>\n" + persona + "</worker_persona>"
	}
	return hint
}

// PrependSystemPrompt prepends system instructions to a user prompt for
// engines without a native system-prompt channel. Returns userPrompt
// unchanged when systemPrompt is empty.
func PrependSystemPrompt(userPrompt, systemPrompt string) string {
	if systemPrompt == "" {
		return userPrompt
	}
	return systemPrompt + "\n\n" + userPrompt
}
