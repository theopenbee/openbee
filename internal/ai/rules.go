package ai

import "fmt"

// BeePersona is the default persona string written to the bee workspace config.
const BeePersona = "You are B, an AI assistant."

// BeeRules returns the system rules preamble for the bee coordinator role.
func BeeRules() string {
	return "You are the coordinator and dispatcher of an AI team. Before processing each user message, you must invoke the Skill tool to load the openbee-bee skill and strictly follow all rules defined in that skill.\n"
}

// WorkerRules returns the system rules for a worker role, embedding optional metadata.
func WorkerRules(name, description, memory string) string {
	rules := "You are a Worker in an AI team, responsible for executing tasks assigned to you. You must invoke the Skill tool to load the openbee-worker skill and strictly follow all rules defined in that skill.\n"
	if name != "" {
		rules += fmt.Sprintf("Name: %s\n", name)
	}
	if description != "" {
		rules += fmt.Sprintf("Description: %s\n", description)
	}
	if memory != "" {
		rules += fmt.Sprintf("\n## Memory Constraints\n%s\n", memory)
	}
	return rules
}

// WorkerPersona returns the persona-only content for a worker's AGENTS.md.
// It contains identity (name, description, memory) but no rule directives.
// Distinct from WorkerRules, which is still used by the Claude engine.
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
// message of a new session for codex/pi engines.
func SkillHintPrefix(role Role) string {
	switch role {
	case RoleBee:
		return "use openbee-bee skill."
	case RoleWorker:
		return "use openbee-worker skill."
	default:
		return ""
	}
}
