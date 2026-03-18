package main

import (
	"fmt"

	"github.com/theopenbee/openbee/internal/app"
	"github.com/theopenbee/openbee/internal/config"
	"github.com/spf13/cobra"
)

var cfgPath string

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "启动 OpenBee 服务",
	RunE: func(cmd *cobra.Command, args []string) error {
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
	serverCmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "配置文件路径")
	rootCmd.AddCommand(serverCmd)
}
