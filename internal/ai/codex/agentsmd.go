package codex

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)

const (
	systemRulesFile = ".openbee.md"
	importLine      = "@" + systemRulesFile
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

func beeRules() string {
	return "You are the coordinator and dispatcher of an AI team. Before processing each user message, you must invoke the Skill tool to load the openbee-bee skill and strictly follow all rules defined in that skill.\n"
}

func workerRules(name, description, memory string) string {
	rules := "You are a Worker in an AI team, responsible for executing tasks assigned to you. You must invoke the Skill tool to load the openbee-worker skill and strictly follow all rules defined in that skill.\n"
	if name != "" {
		rules += fmt.Sprintf("Name: %s\n", name)
	}
	if description != "" {
		rules += fmt.Sprintf("Description: %s\n", description)
	}
	if memory != "" {
		rules += fmt.Sprintf("\n## Memory Constraints\n%s\n", memory)
	}
	return rules
}
