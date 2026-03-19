package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/app"
	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/logger"
)

var cfgPath string

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the OpenBee server",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize with sensible defaults before config is available,
		// so that any log calls during config loading are captured.
		// Level defaults to "info"; format to "json" for log platform compatibility.
		// The log level can be adjusted at runtime via logger.SetLevel() or the HTTP endpoint.
		if err := logger.Init(logger.Config{
			Level:  "info",
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
	rootCmd.AddCommand(serverCmd)
}
