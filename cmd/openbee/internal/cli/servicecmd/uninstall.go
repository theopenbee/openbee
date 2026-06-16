package servicecmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func newUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: i18n.M.Cmd.Service.Uninstall,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}
			if err := mgr.Uninstall(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), i18n.M.Output.Service.Uninstalled)
			return nil
		},
	}
}
