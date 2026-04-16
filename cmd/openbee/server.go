package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/app"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
)

var cfgPath string
var daemonMode bool

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the OpenBee server",
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
		logLevel := os.Getenv("LOG_LEVEL")
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

func init() {
	serverCmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "path to config file")
	serverCmd.Flags().BoolVarP(&daemonMode, "daemon", "d", false, "run as background daemon")
	rootCmd.AddCommand(serverCmd)
}
