package main

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

var ctlSystemCmd = &cobra.Command{Use: "system", Short: ""}

var ctlSystemOverviewCmd = &cobra.Command{
	Use:   "overview",
	Short: "Show system overview: worker status distribution, task stats, recent executions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.GetSystemOverview, nil)
	},
}

var executionsLimit int

var ctlSystemExecutionsCmd = &cobra.Command{
	Use:   "executions",
	Short: "List bee execution history",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{}
		if executionsLimit > 0 {
			a["limit"] = executionsLimit
		}
		return ctlRun(utils.ListBeeExecutions, a)
	},
}

func init() {
	ctlSystemExecutionsCmd.Flags().IntVar(&executionsLimit, "limit", 0, "Number of records to return (0 = server default of 10)")

	ctlSystemCmd.AddCommand(ctlSystemOverviewCmd, ctlSystemExecutionsCmd)
	ctlCmd.AddCommand(ctlSystemCmd)
}
