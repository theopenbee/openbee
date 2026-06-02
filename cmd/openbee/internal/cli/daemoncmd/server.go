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

// NewServerCommand returns the "server" cobra command.
func NewServerCommand() *cobra.Command {
	var (
		cfgPath    string
		daemonMode bool
	)
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
				logFile, err := config.DaemonLogFile()
				if err != nil {
					return err
				}
				// Child: redirect stdout+stderr to log file before logger.Init,
				// so that zap's os.Stderr sink writes to the log file.
				if err := redirectStdio(logFile); err != nil {
					return fmt.Errorf("redirect stdio: %w", err)
				}
				pidFile, err := config.DaemonPIDFile()
				if err != nil {
					return err
				}
				// Clean up PID file on shutdown.
				defer func() { _ = removePIDFile(pidFile) }()
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
