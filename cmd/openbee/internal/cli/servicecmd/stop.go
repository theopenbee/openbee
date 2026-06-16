package servicecmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func newStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: i18n.M.Cmd.Service.Stop,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}
			if err := mgr.Stop(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), i18n.M.Output.Service.Stopped)
			return nil
		},
	}
}
