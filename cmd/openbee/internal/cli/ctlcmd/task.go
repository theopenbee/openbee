package ctlcmd

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

func newTaskCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: i18n.M.Cmd.CtlTask.Short,
	}

	var (
		taskListSessionKey     string
		taskListMessageID      string
		taskListWorkerID       string
		taskListStatus         string
		taskListType           string
		taskListTaskID         string
		taskListPage           int
		taskListPageSize       int
		taskListExecutionLimit int
	)
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks (requires --session-key, --message-id, or --worker-id)",
		RunE: func(c *cobra.Command, args []string) error {
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
			if taskListTaskID != "" {
				a["task_id"] = taskListTaskID
			}
			if taskListPage > 0 {
				a["page"] = taskListPage
			}
			if taskListPageSize > 0 {
				a["page_size"] = taskListPageSize
			}
			if c.Flags().Changed("execution-limit") {
				a["execution_limit"] = taskListExecutionLimit
			}
			return ctlRun(utils.ListTasks, a)
		},
	}
	listCmd.Flags().StringVar(&taskListSessionKey, "session-key", "", "Filter by session key")
	listCmd.Flags().StringVar(&taskListMessageID, "message-id", "", "Filter by message ID")
	listCmd.Flags().StringVar(&taskListWorkerID, "worker-id", "", "Filter by worker ID")
	listCmd.Flags().StringVar(&taskListStatus, "status", "", "Filter by status (comma-separated)")
	listCmd.Flags().StringVar(&taskListType, "type", "", "Filter by type (comma-separated)")
	listCmd.Flags().StringVar(&taskListTaskID, "task-id", "", "Filter by exact task ID")
	listCmd.Flags().IntVar(&taskListPage, "page", 0, "Page number (default: 1)")
	listCmd.Flags().IntVar(&taskListPageSize, "page-size", 0, "Page size (default: 50, max: 100)")
	listCmd.Flags().IntVar(&taskListExecutionLimit, "execution-limit", 0, "Executions per task (default: 10, max: 100; 0 = all for one matching task)")

	var (
		taskCreateMessageID   string
		taskCreateWorkerID    string
		taskCreateInstruction string
		taskCreateType        string
		taskCreateScheduledAt int64
		taskCreateCron        string
	)
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a task",
		RunE: func(c *cobra.Command, args []string) error {
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
	createCmd.Flags().StringVar(&taskCreateMessageID, "message-id", "", "ID of the originating platform message (required)")
	createCmd.Flags().StringVar(&taskCreateWorkerID, "worker-id", "", "Worker ID to assign (required)")
	createCmd.Flags().StringVar(&taskCreateInstruction, "instruction", "", "Instruction for the worker (required)")
	createCmd.Flags().StringVar(&taskCreateType, "type", "", "Task type: immediate, countdown, scheduled (required)")
	createCmd.Flags().Int64Var(&taskCreateScheduledAt, "scheduled-at", 0, "Unix ms; required for countdown type")
	createCmd.Flags().StringVar(&taskCreateCron, "cron", "", "5-field cron expression; required for scheduled type")
	createCmd.MarkFlagRequired("message-id")
	createCmd.MarkFlagRequired("worker-id")
	createCmd.MarkFlagRequired("instruction")
	createCmd.MarkFlagRequired("type")

	cancelCmd := &cobra.Command{
		Use:   "cancel <id>",
		Short: "Cancel a pending or running task",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return ctlRun(utils.CancelTask, map[string]any{"task_id": args[0]})
		},
	}

	cmd.AddCommand(listCmd, createCmd, cancelCmd)
	return cmd
}
