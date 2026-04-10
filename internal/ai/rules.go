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
