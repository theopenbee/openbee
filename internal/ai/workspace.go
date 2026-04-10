package ai

import (
	"fmt"
	"os"
	"path/filepath"
)

// SetupWorkspace initialises the AI engine workspace in workDir by writing the
// AGENTS.md persona file and the system rules file (.openbee.md).
// AGENTS.md is written only on first use (idempotent via CreateFileOnce).
// This is shared by all CLI engines that use the AGENTS.md convention (e.g. Codex, Pi).
func SetupWorkspace(workDir string, role Role, opts WorkspaceOptions) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("mkdir workdir: %w", err)
	}
	switch role {
	case RoleBee:
		if err := createAgentsMD(workDir, BeePersona+"\n"+ImportLine+"\n"); err != nil {
			return err
		}
		return writeSystemRules(workDir, BeeRules())
	case RoleWorker:
		if err := createAgentsMD(workDir, ImportLine+"\n"); err != nil {
			return err
		}
		return writeSystemRules(workDir, WorkerRules(opts.Name, opts.Description, opts.Memory))
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

func writeSystemRules(workDir, content string) error {
	path := filepath.Join(workDir, SystemRulesFile)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", SystemRulesFile, err)
	}
	return nil
}
