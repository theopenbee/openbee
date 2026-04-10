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

// EnsureSystemRules writes .openbee.md with the latest system rules
// for the given role, and ensures CLAUDE.md contains the @import reference.
// It does NOT create CLAUDE.md if it doesn't exist.
func EnsureSystemRules(workDir string, role ai.Role, opts ai.WorkspaceOptions) error {
	var content string
	switch role {
	case ai.RoleBee:
		content = ai.BeeRules()
	case ai.RoleWorker:
		content = ai.WorkerRules(opts.Name, opts.Description, opts.Memory)
	}

	rulesPath := filepath.Join(workDir, ai.SystemRulesFile)
	if err := os.WriteFile(rulesPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", ai.SystemRulesFile, err)
	}

	claudePath := filepath.Join(workDir, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read CLAUDE.md: %w", err)
	}

	if !bytes.Contains(data, []byte(ai.ImportLine)) {
		data = append(data, []byte("\n"+ai.ImportLine+"\n")...)
		if err := os.WriteFile(claudePath, data, 0o644); err != nil {
			return fmt.Errorf("update CLAUDE.md: %w", err)
		}
	}

	return nil
}

// removeImportLine removes the "@.openbee.md" line from CLAUDE.md if present.
// It is a no-op if CLAUDE.md does not exist or does not contain the line.
func removeImportLine(workDir string) error {
	claudePath := filepath.Join(workDir, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read CLAUDE.md: %w", err)
	}

	target := []byte(ai.ImportLine)
	lines := bytes.Split(data, []byte("\n"))
	out := lines[:0]
	for _, line := range lines {
		if !bytes.Equal(bytes.TrimRight(line, "\r"), target) {
			out = append(out, line)
		}
	}
	cleaned := bytes.Join(out, []byte("\n"))
	if bytes.Equal(cleaned, data) {
		return nil // nothing changed
	}
	return os.WriteFile(claudePath, cleaned, 0o644)
}
