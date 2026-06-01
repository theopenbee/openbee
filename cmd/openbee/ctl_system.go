package main

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

var ctlSystemCmd = &cobra.Command{Use: "system", Short: ""}

var ctlSystemOverviewCmd = &cobra.Command{
	Use:   "overview",
	Short: "Show system overview: worker status distribution, task stats",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.GetSystemOverview, nil)
	},
}

func init() {
	ctlSystemCmd.AddCommand(ctlSystemOverviewCmd)
	ctlCmd.AddCommand(ctlSystemCmd)
}
