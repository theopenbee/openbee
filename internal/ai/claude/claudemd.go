package claude

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)

const (
	SystemRulesFile = ai.SystemRulesFile
	ImportLine      = ai.ImportLine
)

// options holds optional parameters for EnsureSystemRules.
type options struct {
	name        string
	description string
	memory      string
}

// Option configures EnsureSystemRules behavior.
type Option func(*options)

// rulesForRole returns the combined rules content for the given role.
func rulesForRole(role ai.Role, opts options) string {
	switch role {
	case ai.RoleBee:
		return ai.BeeRules()
	case ai.RoleWorker:
		return ai.WorkerRules(opts.name, opts.description, opts.memory)
	default:
		return ""
	}
}

// EnsureSystemRules writes .openbee.md with the latest system rules
// for the given role, and ensures CLAUDE.md contains the @import reference.
// It does NOT create CLAUDE.md if it doesn't exist.
func EnsureSystemRules(workDir string, role ai.Role, optFns ...Option) error {
	var opts options
	for _, fn := range optFns {
		fn(&opts)
	}
	rulesPath := filepath.Join(workDir, SystemRulesFile)
	if err := os.WriteFile(rulesPath, []byte(rulesForRole(role, opts)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", SystemRulesFile, err)
	}

	claudePath := filepath.Join(workDir, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read CLAUDE.md: %w", err)
	}

	if !bytes.Contains(data, []byte(ImportLine)) {
		data = append(data, []byte("\n"+ImportLine+"\n")...)
		if err := os.WriteFile(claudePath, data, 0o644); err != nil {
			return fmt.Errorf("update CLAUDE.md: %w", err)
		}
	}

	return nil
}
