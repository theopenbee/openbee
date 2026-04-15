package ai

import "fmt"

// WorkerPersona returns the persona-only content injected into new worker session prompts.
func WorkerPersona(name, description, memory string) string {
	s := "You are a Worker in an AI team.\n"
	if name != "" {
		s += fmt.Sprintf("Name: %s\n", name)
	}
	if description != "" {
		s += fmt.Sprintf("Description: %s\n", description)
	}
	if memory != "" {
		s += fmt.Sprintf("\n## Memory Constraints\n%s\n", memory)
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
		return "[MANDATORY] You MUST invoke the openbee-worker skill immediately using the Skill tool. This is your FIRST and ONLY action before doing anything else. Do NOT skip this step."
	default:
		return ""
	}
}
