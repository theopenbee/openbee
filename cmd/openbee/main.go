package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/theopenbee/openbee/cmd/openbee/internal/cli"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/backupcmd"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/ctlcmd"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/daemoncmd"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/upgradecmd"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/logger"
)

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

func main() {
	// Detect and load language before Execute() so cobra Short/Long fields
	// (set in init()) can be overridden by applyTranslations().
	lang := cli.DetectLang()
	if err := i18n.Load(lang); err != nil {
		fmt.Fprintf(os.Stderr, "warning: i18n load failed: %v\n", err)
	}
	applyTranslations()
	exitCode := func(code int) error { return &cli.ExitCodeError{Code: code} }
	rootCmd.AddCommand(
		daemoncmd.NewServerCommand(),
		daemoncmd.NewStopCommand(exitCode),
		daemoncmd.NewRestartCommand(exitCode),
		daemoncmd.NewStatusCommand(exitCode),
	)
	rootCmd.AddCommand(ctlcmd.NewCommand())
	rootCmd.AddCommand(
		backupcmd.NewBackupCommand(version),
		backupcmd.NewRestoreCommand(version),
	)
	rootCmd.AddCommand(upgradecmd.NewCommand(version))

	if err := rootCmd.Execute(); err != nil {
		var ece *cli.ExitCodeError
		if errors.As(err, &ece) {
			os.Exit(ece.Code)
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
	// Flag descriptions
	configCmd.Flags().Lookup("output").Usage = m.Flag.ConfigOutput
}
