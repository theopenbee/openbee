package ctlcmd

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

func newSystemCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: i18n.M.Cmd.CtlSystem.Short,
	}

	overviewCmd := &cobra.Command{
		Use:   "overview",
		Short: "Show system overview: worker status distribution, task stats",
		RunE: func(c *cobra.Command, args []string) error {
			return ctlRun(utils.GetSystemOverview, nil)
		},
	}
	cmd.AddCommand(overviewCmd)
	return cmd
}
