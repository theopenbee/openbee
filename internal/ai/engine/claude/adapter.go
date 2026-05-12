package claude

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
	core "github.com/theopenbee/openbee/internal/ai/core"
)

const (
	// systemRulesFile is the legacy rules file Claude's Run cleanup removes.
	systemRulesFile = ".openbee.md"
	// importLine is the legacy reference line removed from CLAUDE.md.
	importLine = "@" + systemRulesFile
)

func init() {
	ai.Register(ai.EngineClaude, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineClaude), cfg.ExtraEnv()), nil
	})
}

// claudeAdapter embeds core.BaseAdapter and wraps Run to clean up the legacy
// openbee rules file and matching import line in CLAUDE.md before each run.
type claudeAdapter struct {
	*core.BaseAdapter
}

// NewAdapter constructs a Claude engine adapter.
func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter {
	return &claudeAdapter{
		BaseAdapter: &core.BaseAdapter{
			Invoker:   NewInvoker(binaryPath, extraEnv),
			Collector: NewCollector(),
			Extract:   ExtractResultFromLog,
		},
	}
}

// Run cleans up legacy openbee rules before delegating to the embedded BaseAdapter.
func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	if err := cleanupLegacyRules(workDir); err != nil {
		return ai.RunResult{}, err
	}
	return a.BaseAdapter.Run(ctx, workDir, prompt, opts, logPath)
}

func cleanupLegacyRules(workDir string) error {
	rulesPath := filepath.Join(workDir, systemRulesFile)
	if err := os.Remove(rulesPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", systemRulesFile, err)
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

	target := []byte(importLine)
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
