package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/logger"
)

// exitCodeError is a sentinel error that carries a specific exit code.
// Commands that need a non-zero exit without printing an error message
// (e.g. "status" when the daemon is not running) should return this.
type exitCodeError struct{ code int }

func (e *exitCodeError) Error() string { return "" }

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:     "openbee",
	Short:   "OpenBee core service",
	Version: version,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	rootCmd.SetVersionTemplate(fmt.Sprintf("openbee %s (commit: %s, built: %s)\n", version, commit, date))
}

// resolveExecutable returns the real path of the running binary, following symlinks.
func resolveExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("eval symlinks: %w", err)
	}
	return exe, nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		var ece *exitCodeError
		if errors.As(err, &ece) {
			os.Exit(ece.code)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		logger.Error("fatal", zap.Error(err))
		os.Exit(1)
	}
}
