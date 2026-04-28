package main

import (
	"io"
	"os"

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

var (
	taskDispatchSubParent    string
	taskDispatchSubWorker    string
	taskDispatchSubFromStdin bool
)

var ctlTaskDispatchSubtaskCmd = &cobra.Command{
	Use:   "dispatch-subtask",
	Short: "Group coordinator: create and dispatch a sub-task to a member worker",
	RunE: func(cmd *cobra.Command, args []string) error {
		instruction := ""
		if taskDispatchSubFromStdin {
			b, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			instruction = string(b)
		}
		return ctlRun(utils.DispatchSubtask, map[string]any{
			"parent_task_id": taskDispatchSubParent,
			"worker_id":      taskDispatchSubWorker,
			"instruction":    instruction,
		})
	},
}

var taskSubtasksTaskID string

var ctlTaskSubtasksCmd = &cobra.Command{
	Use:   "subtasks",
	Short: "List all subtasks under a root task",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.ListSubtasks, map[string]any{"task_id": taskSubtasksTaskID})
	},
}

var taskSuspendTaskID string

var ctlTaskSuspendCmd = &cobra.Command{
	Use:   "suspend",
	Short: "Group coordinator: mark the root task waiting_subtasks and exit",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.SuspendTask, map[string]any{"task_id": taskSuspendTaskID})
	},
}

var (
	taskMarkSuccessTaskID string
	taskMarkSuccessStdin  bool
	taskMarkFailedTaskID  string
	taskMarkFailedStdin   bool
)

func readOptionalStdin(flag bool) string {
	if !flag {
		return ""
	}
	b, _ := io.ReadAll(os.Stdin)
	return string(b)
}

var ctlTaskMarkSuccessCmd = &cobra.Command{
	Use:   "mark-success",
	Short: "Group coordinator: declare the root task complete",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.MarkTaskSuccess, map[string]any{
			"task_id": taskMarkSuccessTaskID,
			"result":  readOptionalStdin(taskMarkSuccessStdin),
		})
	},
}

var ctlTaskMarkFailedCmd = &cobra.Command{
	Use:   "mark-failed",
	Short: "Group coordinator: declare the root task failed",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.MarkTaskFailed, map[string]any{
			"task_id": taskMarkFailedTaskID,
			"reason":  readOptionalStdin(taskMarkFailedStdin),
		})
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

	ctlTaskDispatchSubtaskCmd.Flags().StringVar(&taskDispatchSubParent, "parent-task-id", "", "Root task ID (required)")
	ctlTaskDispatchSubtaskCmd.Flags().StringVar(&taskDispatchSubWorker, "worker-id", "", "Member worker ID (required)")
	ctlTaskDispatchSubtaskCmd.Flags().BoolVar(&taskDispatchSubFromStdin, "stdin", false, "Read instruction from stdin")
	ctlTaskDispatchSubtaskCmd.MarkFlagRequired("parent-task-id")
	ctlTaskDispatchSubtaskCmd.MarkFlagRequired("worker-id")

	ctlTaskSubtasksCmd.Flags().StringVar(&taskSubtasksTaskID, "task-id", "", "Root task ID (required)")
	ctlTaskSubtasksCmd.MarkFlagRequired("task-id")

	ctlTaskSuspendCmd.Flags().StringVar(&taskSuspendTaskID, "task-id", "", "Root task ID (required)")
	ctlTaskSuspendCmd.MarkFlagRequired("task-id")

	ctlTaskMarkSuccessCmd.Flags().StringVar(&taskMarkSuccessTaskID, "task-id", "", "Root task ID (required)")
	ctlTaskMarkSuccessCmd.Flags().BoolVar(&taskMarkSuccessStdin, "stdin", false, "Read result from stdin")
	ctlTaskMarkSuccessCmd.MarkFlagRequired("task-id")

	ctlTaskMarkFailedCmd.Flags().StringVar(&taskMarkFailedTaskID, "task-id", "", "Root task ID (required)")
	ctlTaskMarkFailedCmd.Flags().BoolVar(&taskMarkFailedStdin, "stdin", false, "Read failure reason from stdin")
	ctlTaskMarkFailedCmd.MarkFlagRequired("task-id")

	ctlTaskCmd.AddCommand(
		ctlTaskListCmd,
		ctlTaskCreateCmd,
		ctlTaskCancelCmd,
		ctlTaskDispatchSubtaskCmd,
		ctlTaskSubtasksCmd,
		ctlTaskSuspendCmd,
		ctlTaskMarkSuccessCmd,
		ctlTaskMarkFailedCmd,
	)
	ctlCmd.AddCommand(ctlTaskCmd)
}
