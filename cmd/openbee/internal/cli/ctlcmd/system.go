package ctlcmd

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

func newSystemCommand(run Runner) *cobra.Command {
	subs := i18n.M.Cmd.CtlSystem
	cmd := &cobra.Command{
		Use:   "system",
		Short: subs.Short,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "overview",
		Short: subs.Sub("overview"),
		RunE: func(c *cobra.Command, args []string) error {
			return run(utils.GetSystemOverview, nil)
		},
	})
	return cmd
}
