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

// BuildWorkerSessionPrefix returns the Step 1 + Step 2 header for a new worker
// session. When persona is non-empty it is embedded inside <worker_persona>.
func BuildWorkerSessionPrefix(persona string) string {
	var sb strings.Builder
	writePrefixStep1(&sb, "openbee-worker")
	if persona != "" {
		sb.WriteString("After the skill is loaded, internalize the persona below as your identity for the rest of this session:\n\n")
		sb.WriteString("<worker_persona>\n")
		sb.WriteString(persona)
		sb.WriteString("</worker_persona>\n\n")
	}
	sb.WriteString("## Step 2: Execute the task\n")
	return sb.String()
}

// BuildBeeSessionPrefix returns the Step 1 + Step 2 header for a new bee session.
func BuildBeeSessionPrefix() string {
	var sb strings.Builder
	writePrefixStep1(&sb, "openbee-bee")
	sb.WriteString("## Step 2: Handle the messages below\n")
	return sb.String()
}

func writePrefixStep1(sb *strings.Builder, skillName string) {
	sb.WriteString("Please complete the following two steps in order. Do not skip Step 1.\n\n")
	sb.WriteString("## Step 1: Initialize your role\n")
	fmt.Fprintf(sb, "[MANDATORY] You MUST invoke the %s skill immediately, before producing any other output.\n\n", skillName)
}
