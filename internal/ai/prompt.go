package ai

import (
	"fmt"
	"strings"
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

// BuildSessionPrefix returns the Step 1 + Step 2 header for a new session.
// For RoleWorker, persona is embedded as <worker_persona> when non-empty.
// Returns "" for unknown roles.
func BuildSessionPrefix(role Role, persona string) string {
	var skillName, step2Title string
	switch role {
	case RoleWorker:
		skillName = "openbee-worker"
		step2Title = "Execute the task"
	case RoleBee:
		skillName = "openbee-bee"
		step2Title = "Handle the messages below"
	default:
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Please complete the following two steps in order. Do not skip Step 1.\n\n")
	sb.WriteString("## Step 1: Initialize your role\n")
	fmt.Fprintf(&sb, "[MANDATORY] You MUST invoke the %s skill immediately, before producing any other output.\n\n", skillName)

	if role == RoleWorker && persona != "" {
		sb.WriteString("After the skill is loaded, internalize the persona below as your identity for the rest of this session:\n\n")
		sb.WriteString("<worker_persona>\n")
		sb.WriteString(persona)
		sb.WriteString("</worker_persona>\n\n")
	}

	fmt.Fprintf(&sb, "## Step 2: %s\n", step2Title)
	return sb.String()
}
