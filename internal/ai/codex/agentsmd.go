package codex

import (
	"errors"
	"fmt"
	"io/fs"
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

// writeAgentsMD creates workDir/AGENTS.md only if it does not already exist.
func writeAgentsMD(workDir, content string) error {
	path := filepath.Join(workDir, "AGENTS.md")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create AGENTS.md: %w", err)
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

// writeSystemRules always overwrites .openbee.md with the latest rules.
func writeSystemRules(workDir, content string) error {
	path := filepath.Join(workDir, ai.SystemRulesFile)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", ai.SystemRulesFile, err)
	}
	return nil
}
