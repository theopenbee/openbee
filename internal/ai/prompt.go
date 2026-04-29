package ai

import "fmt"

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
		return "[MANDATORY] You MUST invoke the openbee-bee skill immediately using the Skill tool. This is your FIRST and ONLY action before doing anything else. Do NOT skip this step."
	case RoleWorker:
		return "[MANDATORY] You MUST invoke the openbee-worker skill immediately. This is your FIRST and ONLY action before doing anything else. Do NOT skip this step."
	default:
		return ""
	}
}
