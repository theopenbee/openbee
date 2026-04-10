package pi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Invoker spawns pi CLI processes. It is stateless and safe for concurrent use.
type Invoker struct {
	binary   string
	baseEnv  []string
	extraEnv map[string]string
}

// NewInvoker creates an Invoker. extraEnv keys are injected if non-empty.
func NewInvoker(binary, openbeeURL string, extraEnv map[string]string) *Invoker {
	return &Invoker{
		binary:   binary,
		baseEnv:  ai.BuildBaseEnv(openbeeURL),
		extraEnv: extraEnv,
	}
}

// ExtractResultFromLog is implemented in Task 3.
func ExtractResultFromLog(logPath string) string { return "" }

// Run is implemented in Task 4.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return nil, nil, fmt.Errorf("not implemented")
}

// sessionDir returns the directory where pi session files are stored.
func sessionDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}
	return filepath.Join(home, ".openbee", ".pi", "sessions"), nil
}
