package core

import (
	"fmt"
	"strings"

	"github.com/theopenbee/openbee/internal/ai"
)

// WorkerIdentity describes a worker's identity to embed in the <worker_persona>
// block. The zero value yields no persona block.
type WorkerIdentity struct {
	Name        string
	Description string
	Constraints string
}

// SessionRequest is the complete input required to build a session prompt.
// Identity is only consulted when Role is ai.RoleWorker.
type SessionRequest struct {
	Role     ai.Role
	Identity WorkerIdentity
	Resume   bool
	Content  string
}

// BuildSessionPrompt returns a full session prompt. When Resume is true it
// returns Content verbatim (no prefix). Otherwise it prepends a role-specific
// prefix (Step 1 + optional persona block + Step 2 header) to Content.
func BuildSessionPrompt(req SessionRequest) string {
	if req.Resume {
		return req.Content
	}
	return buildSessionPrefix(req.Role, req.Identity.persona()) + req.Content
}

// BuildBeeSessionPrompt is a role-fixed convenience for the bee path so
// business callers do not need to import internal/ai for the Role enum.
func BuildBeeSessionPrompt(resume bool, content string) string {
	return BuildSessionPrompt(SessionRequest{Role: ai.RoleBee, Resume: resume, Content: content})
}

// BuildWorkerSessionPrompt is a role-fixed convenience for the worker path so
// business callers do not need to import internal/ai for the Role enum.
func BuildWorkerSessionPrompt(identity WorkerIdentity, resume bool, content string) string {
	return BuildSessionPrompt(SessionRequest{Role: ai.RoleWorker, Identity: identity, Resume: resume, Content: content})
}

type sessionPrefixSpec struct {
	skillName   string
	step2Header string
	personaTag  string // empty string means "this role does not support persona"
}

var rolePrefixSpecs = map[ai.Role]sessionPrefixSpec{
	ai.RoleWorker: {
		skillName:   "openbee-worker",
		step2Header: "## Step 2: Execute the task\n",
		personaTag:  "worker_persona",
	},
	ai.RoleBee: {
		// Trailing "\n\n" preserves the blank line that used to sit between
		// the bee step 2 header and the first <message_meta> entry.
		skillName:   "openbee-bee",
		step2Header: "## Step 2: Handle the messages below\n\n",
		personaTag:  "",
	},
}

func buildSessionPrefix(role ai.Role, persona string) string {
	spec := rolePrefixSpecs[role]
	var sb strings.Builder
	sb.WriteString("Please complete the following two steps in order. Do not skip Step 1.\n\n")
	sb.WriteString("## Step 1: Initialize your role\n")
	fmt.Fprintf(&sb, "[MANDATORY] You MUST invoke the %s skill immediately, before producing any other output.\n\n", spec.skillName)
	if persona != "" && spec.personaTag != "" {
		sb.WriteString("After the skill is loaded, internalize the persona below as your identity for the rest of this session:\n\n")
		fmt.Fprintf(&sb, "<%s>\n", spec.personaTag)
		sb.WriteString(persona)
		fmt.Fprintf(&sb, "</%s>\n\n", spec.personaTag)
	}
	sb.WriteString(spec.step2Header)
	return sb.String()
}

// persona converts a WorkerIdentity into the body that is wrapped inside the
// <worker_persona> block. Returns "" when the identity is the zero value.
func (id WorkerIdentity) persona() string {
	if id.Name == "" && id.Description == "" && id.Constraints == "" {
		return ""
	}
	s := "## Role\nYou are a Worker in an AI team.\n"
	if id.Name != "" || id.Description != "" {
		s += "\n## Identity\n"
	}
	if id.Name != "" {
		s += fmt.Sprintf("Name: %s\n", id.Name)
	}
	if id.Description != "" {
		s += fmt.Sprintf("Description: %s\n", id.Description)
	}
	if id.Constraints != "" {
		s += fmt.Sprintf("\n## Work Constraints\n%s\n", id.Constraints)
	}
	return s
}
