package daemoncmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/app"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/logger"
)

var cfgPath string
var daemonMode bool

// NewServerCommand returns the "server" cobra command.
func NewServerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: i18n.M.Cmd.Server.Short,
		RunE: func(cmd *cobra.Command, args []string) error {
			// --- Daemon dispatch ---
			child := isDaemonChild()
			if daemonMode && !child {
				// Parent: spawn background child and exit.
				return daemonize(cfgPath)
			}

			if child {
				// Child: redirect stdout+stderr to log file before logger.Init,
				// so that zap's os.Stderr sink writes to the log file.
				if err := redirectStdio(daemonLogFile()); err != nil {
					return fmt.Errorf("redirect stdio: %w", err)
				}
				// Clean up PID file on shutdown.
				defer func() { _ = removePIDFile() }()
			}

			// --- Normal server startup ---
			logLevel := os.Getenv("OPENBEE_LOG_LEVEL")
			if logLevel == "" {
				logLevel = "info"
			}
			if err := logger.Init(logger.Config{
				Level:  logLevel,
				Format: "json",
			}); err != nil {
				return fmt.Errorf("init logger: %w", err)
			}
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			a, err := app.BuildApp(cfg)
			if err != nil {
				return fmt.Errorf("build app: %w", err)
			}

			a.Run()
			return nil
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", i18n.M.Flag.ConfigPath)
	cmd.Flags().BoolVarP(&daemonMode, "daemon", "d", false, i18n.M.Flag.ServerDaemon)
	return cmd
}
