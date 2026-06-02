package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/theopenbee/openbee/cmd/openbee/internal/cli"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/logger"
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
	// Detect and load language before Execute() so cobra Short/Long fields
	// (set in init()) can be overridden by applyTranslations().
	lang := cli.DetectLang()
	if err := i18n.Load(lang); err != nil {
		fmt.Fprintf(os.Stderr, "warning: i18n load failed: %v\n", err)
	}
	applyTranslations()

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

// applyTranslations overwrites cobra command Short/Long fields with the
// loaded locale. Must be called after i18n.Load() and before Execute().
func applyTranslations() {
	m := i18n.M
	rootCmd.Short = m.Cmd.Root.Short
	configCmd.Short = m.Cmd.Config.Short
	serverCmd.Short = m.Cmd.Server.Short
	stopCmd.Short = m.Cmd.Stop.Short
	restartCmd.Short = m.Cmd.Restart.Short
	statusCmd.Short = m.Cmd.Status.Short
	upgradeCmd.Short = m.Cmd.Upgrade.Short
	upgradeCmd.Long = m.Cmd.Upgrade.Long
	backupCmd.Short = m.Cmd.Backup.Short
	restoreCmd.Short = m.Cmd.Restore.Short
	// Flag descriptions
	serverCmd.Flags().Lookup("config").Usage = m.Flag.ConfigPath
	serverCmd.Flags().Lookup("daemon").Usage = m.Flag.ServerDaemon
	configCmd.Flags().Lookup("output").Usage = m.Flag.ConfigOutput
	backupCmd.Flags().Lookup("config").Usage = m.Flag.ConfigPath
	backupCmd.Flags().Lookup("password").Usage = m.Flag.BackupPassword
	restoreCmd.Flags().Lookup("config").Usage = m.Flag.ConfigPath
	restoreCmd.Flags().Lookup("password").Usage = m.Flag.RestorePassword
	restoreCmd.Flags().Lookup("force").Usage = m.Flag.RestoreForce
	restartCmd.Flags().Lookup("config").Usage = m.Flag.ConfigPath
	upgradeCmd.Flags().Lookup("check").Usage = m.Flag.UpgradeCheck

	applyCtlTranslations()
}
