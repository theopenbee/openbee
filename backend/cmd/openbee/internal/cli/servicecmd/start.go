package servicecmd

import (
	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/config"
)

func newStartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the service via the OS service manager",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}
			if err := mgr.Start(cmd.Context()); err != nil {
				return err
			}
			logPath, _ := config.DaemonLogFile()
			return reportRunStateAfterStart(cmd.Context(), mgr, cmd.OutOrStdout(), logPath)
		},
	}
}
