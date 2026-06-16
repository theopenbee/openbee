package servicecmd

import (
	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: i18n.M.Cmd.Service.Short,
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

