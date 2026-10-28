package servicecmd

import (
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage openbee user-level autostart",
	}
	cmd.AddCommand(
		newInstallCommand(),
		newUninstallCommand(),
		newStartCommand(),
		newStopCommand(),
		newStatusCommand(),
	)
	return cmd
}
