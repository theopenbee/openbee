package codex

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)

var (
	systemRulesFile = ai.SystemRulesFile
	importLine      = ai.ImportLine
)

func setupWorkspace(workDir string, role ai.Role, opts ai.WorkspaceOptions) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("mkdir workdir: %w", err)
	}
	switch role {
	case ai.RoleBee:
		if err := writeAgentsMD(workDir, "You are B, an AI assistant.\n"+importLine+"\n"); err != nil {
			return err
		}
		return writeSystemRules(workDir, beeRules())
	case ai.RoleWorker:
		if err := writeAgentsMD(workDir, importLine+"\n"); err != nil {
			return err
		}
		return writeSystemRules(workDir, workerRules(opts.Name, opts.Description, opts.Memory))
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
	path := filepath.Join(workDir, systemRulesFile)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", systemRulesFile, err)
	}
	return nil
}

func beeRules() string     { return ai.BeeRules() }
func workerRules(name, description, memory string) string {
	return ai.WorkerRules(name, description, memory)
}
