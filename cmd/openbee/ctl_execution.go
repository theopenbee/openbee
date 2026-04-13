package main

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

var ctlExecutionCmd = &cobra.Command{Use: "execution", Short: ""}

var (
	execListWorkerID      string
	execListSessionID     string
	execListStatus        string
	execListStartedFrom   int64
	execListStartedTo     int64
	execListCompletedFrom int64
	execListCompletedTo   int64
)

var ctlExecutionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List worker executions with optional filters",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{}
		if execListWorkerID != "" {
			a["worker_id"] = execListWorkerID
		}
		if execListSessionID != "" {
			a["session_id"] = execListSessionID
		}
		if execListStatus != "" {
			a["status"] = execListStatus
		}
		if execListStartedFrom > 0 {
			a["started_at_from"] = execListStartedFrom
		}
		if execListStartedTo > 0 {
			a["started_at_to"] = execListStartedTo
		}
		if execListCompletedFrom > 0 {
			a["completed_at_from"] = execListCompletedFrom
		}
		if execListCompletedTo > 0 {
			a["completed_at_to"] = execListCompletedTo
		}
		return ctlRun(utils.ListExecutions, a)
	},
}

func init() {
	ctlExecutionListCmd.Flags().StringVar(&execListWorkerID, "worker-id", "", "Filter by worker ID")
	ctlExecutionListCmd.Flags().StringVar(&execListSessionID, "session-id", "", "Filter by session ID")
	ctlExecutionListCmd.Flags().StringVar(&execListStatus, "status", "", "Filter by status (pending, running, completed, failed)")
	ctlExecutionListCmd.Flags().Int64Var(&execListStartedFrom, "started-from", 0, "Filter started_at >= value (Unix ms)")
	ctlExecutionListCmd.Flags().Int64Var(&execListStartedTo, "started-to", 0, "Filter started_at <= value (Unix ms)")
	ctlExecutionListCmd.Flags().Int64Var(&execListCompletedFrom, "completed-from", 0, "Filter completed_at >= value (Unix ms)")
	ctlExecutionListCmd.Flags().Int64Var(&execListCompletedTo, "completed-to", 0, "Filter completed_at <= value (Unix ms)")

	ctlExecutionCmd.AddCommand(ctlExecutionListCmd)
	ctlCmd.AddCommand(ctlExecutionCmd)
}
