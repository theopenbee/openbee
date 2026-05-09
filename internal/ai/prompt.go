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

// BuildSessionPrefix returns the Step-1 + Step-2 header for a new session.
// The trailing "## Step 2: ...\n" line ends with a newline so the caller can
// append the task body directly without inserting a separator.
//
//	role    — RoleWorker or RoleBee. Selects the skill name and Step 2 title.
//	persona — Worker persona body produced by WorkerPersona(). Pass "" for Bee
//	          or when no worker record is available. The <worker_persona> block
//	          is emitted only when role == RoleWorker and persona != ""; for
//	          RoleBee any non-empty persona is intentionally ignored.
//
// For unknown roles the function returns "" so callers that check for an empty
// prefix keep working without special-casing unknown roles.
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

	s := "Please complete the following two steps in order. Do not skip Step 1.\n\n"
	s += "## Step 1: Initialize your role\n"
	s += fmt.Sprintf("[MANDATORY] You MUST invoke the %s skill immediately, before producing any other output.", skillName)

	if role == RoleWorker && persona != "" {
		s += " After the skill is loaded, internalize the persona below as your identity for the rest of this session:\n\n"
		s += "<worker_persona>\n" + persona + "</worker_persona>\n\n"
	} else {
		s += "\n\n"
	}

	s += fmt.Sprintf("## Step 2: %s\n", step2Title)
	return s
}
