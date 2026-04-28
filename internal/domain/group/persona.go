package group

import (
	"fmt"
	"strings"

	"github.com/theopenbee/openbee/internal/infra/model"
)

// BuildPersona builds the persona block injected into the Group agent's prompt.
// It MUST be deterministic given the same inputs.
func BuildPersona(g model.Group, members []model.MemberBrief) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Name: %s\n", g.Name)
	if g.Description != "" {
		fmt.Fprintf(&sb, "Description: %s\n", g.Description)
	}
	if g.Constraints != "" {
		fmt.Fprintf(&sb, "\n## Work Constraints\n%s\n", g.Constraints)
	}
	sb.WriteString("\n## Members\n")
	if len(members) == 0 {
		sb.WriteString("(no members)\n")
	} else {
		for _, m := range members {
			fmt.Fprintf(&sb, "- id=%s name=%s desc=%s\n", m.ID, m.Name, m.Description)
		}
	}
	sb.WriteString("\n## Coordinator Protocol\n")
	sb.WriteString("Use these CLI commands to coordinate sub-tasks:\n")
	sb.WriteString("  openbee ctl task dispatch-subtask --parent-task-id <root> --worker-id <w> --stdin\n")
	sb.WriteString("  openbee ctl task subtasks       --task-id <root>\n")
	sb.WriteString("  openbee ctl task suspend        --task-id <root>\n")
	sb.WriteString("  openbee ctl task mark-success   --task-id <root> [--stdin]\n")
	sb.WriteString("  openbee ctl task mark-failed    --task-id <root> [--stdin]\n")
	sb.WriteString("Each turn: take ONE action set, then call `task suspend` to await sub-task events.\n")
	return sb.String()
}
