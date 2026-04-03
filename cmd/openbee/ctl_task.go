package main

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

var ctlTaskCmd = &cobra.Command{Use: "task", Short: ""}

var (
	taskListSessionKey string
	taskListMessageID  string
	taskListWorkerID   string
	taskListStatus     string
	taskListType       string
)

var ctlTaskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks (requires --session-key, --message-id, or --worker-id)",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{}
		if taskListSessionKey != "" {
			a["session_key"] = taskListSessionKey
		}
		if taskListMessageID != "" {
			a["message_id"] = taskListMessageID
		}
		if taskListWorkerID != "" {
			a["worker_id"] = taskListWorkerID
		}
		if taskListStatus != "" {
			a["status"] = taskListStatus
		}
		if taskListType != "" {
			a["type"] = taskListType
		}
		return ctlRun(utils.ListTasks, a)
	},
}

var (
	taskCreateMessageID   string
	taskCreateWorkerID    string
	taskCreateInstruction string
	taskCreateType        string
	taskCreateScheduledAt int64
	taskCreateCron        string
)

var ctlTaskCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a task",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{
			"message_id":  taskCreateMessageID,
			"worker_id":   taskCreateWorkerID,
			"instruction": taskCreateInstruction,
			"type":        taskCreateType,
		}
		if taskCreateScheduledAt != 0 {
			a["scheduled_at"] = taskCreateScheduledAt
		}
		if taskCreateCron != "" {
			a["cron_expr"] = taskCreateCron
		}
		return ctlRun(utils.CreateTask, a)
	},
}

var ctlTaskCancelCmd = &cobra.Command{
	Use:   "cancel <id>",
	Short: "Cancel a pending or scheduled task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.CancelTask, map[string]any{"task_id": args[0]})
	},
}

func init() {
	ctlTaskListCmd.Flags().StringVar(&taskListSessionKey, "session-key", "", "Filter by session key")
	ctlTaskListCmd.Flags().StringVar(&taskListMessageID, "message-id", "", "Filter by message ID")
	ctlTaskListCmd.Flags().StringVar(&taskListWorkerID, "worker-id", "", "Filter by worker ID")
	ctlTaskListCmd.Flags().StringVar(&taskListStatus, "status", "", "Filter by status (comma-separated)")
	ctlTaskListCmd.Flags().StringVar(&taskListType, "type", "", "Filter by type (comma-separated)")

	ctlTaskCreateCmd.Flags().StringVar(&taskCreateMessageID, "message-id", "", "ID of the originating platform message (required)")
	ctlTaskCreateCmd.Flags().StringVar(&taskCreateWorkerID, "worker-id", "", "Worker ID to assign (required)")
	ctlTaskCreateCmd.Flags().StringVar(&taskCreateInstruction, "instruction", "", "Instruction for the worker (required)")
	ctlTaskCreateCmd.Flags().StringVar(&taskCreateType, "type", "", "Task type: immediate, countdown, scheduled (required)")
	ctlTaskCreateCmd.Flags().Int64Var(&taskCreateScheduledAt, "scheduled-at", 0, "Unix ms; required for countdown type")
	ctlTaskCreateCmd.Flags().StringVar(&taskCreateCron, "cron", "", "5-field cron expression; required for scheduled type")
	ctlTaskCreateCmd.MarkFlagRequired("message-id")
	ctlTaskCreateCmd.MarkFlagRequired("worker-id")
	ctlTaskCreateCmd.MarkFlagRequired("instruction")
	ctlTaskCreateCmd.MarkFlagRequired("type")

	ctlTaskCmd.AddCommand(ctlTaskListCmd, ctlTaskCreateCmd, ctlTaskCancelCmd)
	ctlCmd.AddCommand(ctlTaskCmd)
}
