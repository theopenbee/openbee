package main

import (
	"github.com/spf13/cobra"
)

var restartCfgPath string

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the OpenBee daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Stop the existing daemon (tolerates not-running).
		if err := doStop(daemonPIDFile()); err != nil {
			return err
		}
		// Spawn a fresh daemon with the given config.
		return daemonize(restartCfgPath)
	},
}

func init() {
	restartCmd.Flags().StringVarP(&restartCfgPath, "config", "c", "config.yaml", "path to config file")
	rootCmd.AddCommand(restartCmd)
}
