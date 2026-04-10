package pi

import (
	"fmt"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func setupWorkspace(workDir string, role ai.Role, opts ai.WorkspaceOptions) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("mkdir workdir: %w", err)
	}
	switch role {
	case ai.RoleBee:
		if err := writeAgentsMD(workDir, ai.BeePersona+"\n"+ai.ImportLine+"\n"); err != nil {
			return err
		}
		return writeSystemRules(workDir, ai.BeeRules())
	case ai.RoleWorker:
		if err := writeAgentsMD(workDir, ai.ImportLine+"\n"); err != nil {
			return err
		}
		return writeSystemRules(workDir, ai.WorkerRules(opts.Name, opts.Description, opts.Memory))
	default:
		return fmt.Errorf("unknown role: %q", role)
	}
}

func writeAgentsMD(workDir, content string) error {
	if err := ai.CreateFileOnce(filepath.Join(workDir, "AGENTS.md"), content); err != nil {
		return fmt.Errorf("create AGENTS.md: %w", err)
	}
	return nil
}

func writeSystemRules(workDir, content string) error {
	path := filepath.Join(workDir, ai.SystemRulesFile)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", ai.SystemRulesFile, err)
	}
	return nil
}
