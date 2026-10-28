package ctlcmd

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

func newSystemCommand(run Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "View system status overview",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "overview",
		Short: "Show system overview: worker status distribution, task stats",
		RunE: func(c *cobra.Command, args []string) error {
			return run(utils.GetSystemOverview, nil)
		},
	})
	return cmd
}
