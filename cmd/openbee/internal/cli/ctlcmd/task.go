package ctlcmd

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

func newTaskCommand(run Runner) *cobra.Command {
	subs := i18n.M.Cmd.CtlTask
	cmd := &cobra.Command{
		Use:   "task",
		Short: subs.Short,
	}
	cmd.AddCommand(
		newTaskListCommand(run, subs.Sub("list")),
		newTaskCreateCommand(run, subs.Sub("create")),
		newTaskCancelCommand(run, subs.Sub("cancel")),
	)
	return cmd
}

func newTaskListCommand(run Runner, short string) *cobra.Command {
	var (
		sessionKey     string
		messageID      string
		workerID       string
		status         string
		typeFilter     string
		taskID         string
		page           int
		pageSize       int
		executionLimit int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: short,
		RunE: func(c *cobra.Command, args []string) error {
			a := map[string]any{}
			setIfNonEmpty(a, "session_key", sessionKey)
			setIfNonEmpty(a, "message_id", messageID)
			setIfNonEmpty(a, "worker_id", workerID)
			setIfNonEmpty(a, "status", status)
			setIfNonEmpty(a, "type", typeFilter)
			setIfNonEmpty(a, "task_id", taskID)
			setIfPositive(a, "page", page)
			setIfPositive(a, "page_size", pageSize)
			setIfFlagChanged(c, a, "execution-limit", "execution_limit", executionLimit)
			return run(utils.ListTasks, a)
		},
	}
	cmd.Flags().StringVar(&sessionKey, "session-key", "", "Filter by session key")
	cmd.Flags().StringVar(&messageID, "message-id", "", "Filter by message ID")
	cmd.Flags().StringVar(&workerID, "worker-id", "", "Filter by worker ID")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status (comma-separated)")
	cmd.Flags().StringVar(&typeFilter, "type", "", "Filter by type (comma-separated)")
	cmd.Flags().StringVar(&taskID, "task-id", "", "Filter by exact task ID")
	cmd.Flags().IntVar(&page, "page", 0, "Page number (default: 1)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "Page size (default: 50, max: 100)")
	cmd.Flags().IntVar(&executionLimit, "execution-limit", 0, "Executions per task (default: 10, max: 100; 0 = all for one matching task)")
	return cmd
}

func newTaskCreateCommand(run Runner, short string) *cobra.Command {
	var (
		messageID   string
		workerID    string
		instruction string
		typeName    string
		scheduledAt int64
		cron        string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: short,
		RunE: func(c *cobra.Command, args []string) error {
			a := map[string]any{
				"message_id":  messageID,
				"worker_id":   workerID,
				"instruction": instruction,
				"type":        typeName,
			}
			setIfPositiveInt64(a, "scheduled_at", scheduledAt)
			setIfNonEmpty(a, "cron_expr", cron)
			return run(utils.CreateTask, a)
		},
	}
	cmd.Flags().StringVar(&messageID, "message-id", "", "ID of the originating platform message (required)")
	cmd.Flags().StringVar(&workerID, "worker-id", "", "Worker ID to assign (required)")
	cmd.Flags().StringVar(&instruction, "instruction", "", "Instruction for the worker (required)")
	cmd.Flags().StringVar(&typeName, "type", "", "Task type: immediate, countdown, scheduled (required)")
	cmd.Flags().Int64Var(&scheduledAt, "scheduled-at", 0, "Unix ms; required for countdown type")
	cmd.Flags().StringVar(&cron, "cron", "", "5-field cron expression; required for scheduled type")
	cmd.MarkFlagRequired("message-id")
	cmd.MarkFlagRequired("worker-id")
	cmd.MarkFlagRequired("instruction")
	cmd.MarkFlagRequired("type")
	return cmd
}

func newTaskCancelCommand(run Runner, short string) *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return run(utils.CancelTask, map[string]any{"task_id": args[0]})
		},
	}
}
