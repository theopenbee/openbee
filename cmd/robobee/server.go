package main

import (
	"log/slog"
	"os"

	"github.com/robobee/core/internal/app"
	"github.com/robobee/core/internal/config"
	"github.com/spf13/cobra"
)

var cfgPath string

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "启动 RoboBee 服务",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			slog.Error("failed to load config", "error", err)
			os.Exit(1)
		}

		a, err := app.BuildApp(cfg)
		if err != nil {
			slog.Error("failed to build app", "error", err)
			os.Exit(1)
		}

		a.Run()
		return nil
	},
}

func init() {
	serverCmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "配置文件路径")
	rootCmd.AddCommand(serverCmd)
}
