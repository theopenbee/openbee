package servicecmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func newStartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: i18n.M.Cmd.Service.Start,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}
			if err := mgr.Start(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), i18n.M.Output.Service.Started)
			return nil
		},
	}
}
