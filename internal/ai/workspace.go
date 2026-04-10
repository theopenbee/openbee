package ai

import (
	"fmt"
	"os"
	"path/filepath"
)

// LoadInstruction is the mandatory directive written to AGENTS.md for engines
// that do not support @import syntax (e.g. Codex, Pi). It instructs the agent
// to explicitly read .openbee.md before each task.
const LoadInstruction = "Before starting each task, you MUST read the file " + SystemRulesFile + " and strictly follow all rules defined in it."

// WorkspaceOptions carries per-agent metadata used during workspace initialisation.
type WorkspaceOptions struct {
	Name        string
	Description string
	Memory      string
}

// SetupWorkspace initialises the AI engine workspace in workDir by writing the
// AGENTS.md persona file. No system rules file (.openbee.md) is written;
// rule injection is handled via the skill hint prefix on new sessions.
// This is shared by all CLI engines that use the AGENTS.md convention (e.g. Codex, Pi).
func SetupWorkspace(workDir string, role Role, opts WorkspaceOptions) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("mkdir workdir: %w", err)
	}
	switch role {
	case RoleBee:
		return createAgentsMD(workDir, BeePersona+"\n")
	case RoleWorker:
		return createAgentsMD(workDir, WorkerPersona(opts.Name, opts.Description, opts.Memory))
	default:
		return fmt.Errorf("unknown role: %q", role)
	}
}

func createAgentsMD(workDir, content string) error {
	if err := CreateFileOnce(filepath.Join(workDir, "AGENTS.md"), content); err != nil {
		return fmt.Errorf("create AGENTS.md: %w", err)
	}
	return nil
}
