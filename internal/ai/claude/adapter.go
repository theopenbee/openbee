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

type claudeAdapter struct {
	invoker   *Invoker
	collector *Collector
}

func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter {
	return &claudeAdapter{
		invoker:   NewInvoker(binaryPath, extraEnv),
		collector: NewCollector(),
	}
}

func (a *claudeAdapter) Prepare(workDir string, _ ai.PrepareOptions) error {
	rulesPath := filepath.Join(workDir, SystemRulesFile)
	if err := os.Remove(rulesPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", SystemRulesFile, err)
	}
	return removeImportLine(workDir)
}

func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := a.invoker.Run(ctx, workDir, prompt, opts, logPath)
	return ai.NewRunResult(proc, out, err, func() string {
		return ExtractResultFromLog(logPath)
	})
}

func (a *claudeAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return a.collector.Collect(ctx, sessionID)
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
