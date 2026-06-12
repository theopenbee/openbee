package servicecmd

import (
	"github.com/spf13/cobra"
)

// NewCommand returns the "service" cobra command group.
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

func newInstallCommand() *cobra.Command   { return &cobra.Command{Use: "install"} }
func newUninstallCommand() *cobra.Command { return &cobra.Command{Use: "uninstall"} }
func newStartCommand() *cobra.Command     { return &cobra.Command{Use: "start"} }
func newStopCommand() *cobra.Command      { return &cobra.Command{Use: "stop"} }
func newStatusCommand() *cobra.Command    { return &cobra.Command{Use: "status"} }
