package servicecmd

import (
	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

// NewCommand returns the "service" cobra command group.
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

func newInstallCommand() *cobra.Command   { return &cobra.Command{Use: "install"} }
func newUninstallCommand() *cobra.Command { return &cobra.Command{Use: "uninstall"} }
func newStartCommand() *cobra.Command     { return &cobra.Command{Use: "start"} }
func newStopCommand() *cobra.Command      { return &cobra.Command{Use: "stop"} }
func newStatusCommand() *cobra.Command    { return &cobra.Command{Use: "status"} }
