package claude

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)

const (
	SystemRulesFile = ".openbee.md"
	ImportLine      = "@" + SystemRulesFile
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
		return beeRules()
	case ai.RoleWorker:
		return workerRules(opts.name, opts.description, opts.memory)
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
	// 1. Write .openbee.md (always overwrite)
	rulesPath := filepath.Join(workDir, SystemRulesFile)
	if err := os.WriteFile(rulesPath, []byte(rulesForRole(role, opts)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", SystemRulesFile, err)
	}

	// 2. Check CLAUDE.md for import reference
	claudePath := filepath.Join(workDir, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // CLAUDE.md doesn't exist, skip
		}
		return fmt.Errorf("read CLAUDE.md: %w", err)
	}

	// 3. Append import if missing
	if !bytes.Contains(data, []byte(ImportLine)) {
		data = append(data, []byte("\n"+ImportLine+"\n")...)
		if err := os.WriteFile(claudePath, data, 0o644); err != nil {
			return fmt.Errorf("update CLAUDE.md: %w", err)
		}
	}

	return nil
}
