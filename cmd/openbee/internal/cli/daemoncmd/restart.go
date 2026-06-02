package daemoncmd

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

// NewRestartCommand returns the "restart" cobra command.
func NewRestartCommand(exitCode ExitCodeFunc) *cobra.Command {
	var restartCfgPath string
	cmd := &cobra.Command{
		Use:   "restart",
		Short: i18n.M.Cmd.Restart.Short,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Stop the existing daemon (tolerates not-running).
			if err := doStop(daemonPIDFile()); err != nil {
				return err
			}
			// Spawn a fresh daemon with the given config.
			return daemonize(restartCfgPath)
		},
	}
	cmd.Flags().StringVarP(&restartCfgPath, "config", "c", "config.yaml", i18n.M.Flag.ConfigPath)
	return cmd
}
