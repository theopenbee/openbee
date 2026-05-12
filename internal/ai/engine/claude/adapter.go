package claude

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
	core "github.com/theopenbee/openbee/internal/ai/core"
)

const (
	// SystemRulesFile is the legacy rules file Claude's Prepare cleans up.
	SystemRulesFile = ".openbee.md"
	// ImportLine is the legacy reference line removed from CLAUDE.md.
	ImportLine = "@" + SystemRulesFile
)

func init() {
	ai.Register(ai.EngineClaude, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineClaude), cfg.ExtraEnv()), nil
	})
}

// claudeAdapter embeds core.BaseAdapter and overrides Prepare to clean up the
// legacy openbee rules file and matching import line in CLAUDE.md.
type claudeAdapter struct {
	*core.BaseAdapter
}

// NewAdapter constructs a Claude engine adapter with Claude-specific Prepare.
func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter {
	return &claudeAdapter{
		BaseAdapter: &core.BaseAdapter{
			Invoker:   NewInvoker(binaryPath, extraEnv),
			Collector: NewCollector(),
			Extract:   ExtractResultFromLog,
		},
	}
}

// Prepare removes the legacy .openbee.md rules file and its import line from CLAUDE.md.
func (a *claudeAdapter) Prepare(workDir string, _ ai.PrepareOptions) error {
	rulesPath := filepath.Join(workDir, SystemRulesFile)
	if err := os.Remove(rulesPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", SystemRulesFile, err)
	}
	return removeImportLine(workDir)
}

func removeImportLine(workDir string) error {
	claudePath := filepath.Join(workDir, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read CLAUDE.md: %w", err)
	}

	target := []byte(ImportLine)
	lines := bytes.Split(data, []byte("\n"))
	out := lines[:0]
	for _, line := range lines {
		if !bytes.Equal(bytes.TrimRight(line, "\r"), target) {
			out = append(out, line)
		}
	}
	cleaned := bytes.Join(out, []byte("\n"))
	if bytes.Equal(cleaned, data) {
		return nil
	}
	return os.WriteFile(claudePath, cleaned, 0o644)
}
