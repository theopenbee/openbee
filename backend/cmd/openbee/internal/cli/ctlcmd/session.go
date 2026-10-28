package ctlcmd

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

func newSessionCommand(run Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage session contexts",
	}
	cmd.AddCommand(
		newSessionListCommand(run, "List all agents with active session contexts for a session key"),
		newSessionClearCommand(run, "Cancel all active tasks and clear all session contexts for a session key"),
		newSessionClearWorkerCommand(run, "Cancel one worker's active tasks and reset its session context within a session"),
	)
	return cmd
}

func newSessionListCommand(run Runner, short string) *cobra.Command {
	var sessionKey string
	cmd := &cobra.Command{
		Use:   "list",
		Short: short,
		RunE: func(c *cobra.Command, args []string) error {
			return run(utils.ListSessionContexts, map[string]any{"session_key": sessionKey})
		},
	}
	cmd.Flags().StringVar(&sessionKey, "session-key", "", "Session key to query (required)")
	cmd.MarkFlagRequired("session-key")
	return cmd
}

func newSessionClearCommand(run Runner, short string) *cobra.Command {
	var (
		sessionKey string
		force      bool
	)
	cmd := &cobra.Command{
		Use:   "clear",
		Short: short,
		RunE: func(c *cobra.Command, args []string) error {
			return run(utils.ClearSession, map[string]any{
				"session_key": sessionKey,
				"force":       force,
			})
		},
	}
	cmd.Flags().StringVar(&sessionKey, "session-key", "", "Session key to clear (required)")
	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation when multiple workers are linked")
	cmd.MarkFlagRequired("session-key")
	return cmd
}

func newSessionClearWorkerCommand(run Runner, short string) *cobra.Command {
	var (
		sessionKey string
		workerID   string
		force      bool
	)
	cmd := &cobra.Command{
		Use:   "clear-worker",
		Short: short,
		RunE: func(c *cobra.Command, args []string) error {
			return run(utils.ClearWorkerSession, map[string]any{
				"session_key": sessionKey,
				"worker_id":   workerID,
				"force":       force,
			})
		},
	}
	cmd.Flags().StringVar(&sessionKey, "session-key", "", "Session key (required)")
	cmd.Flags().StringVar(&workerID, "worker-id", "", "Worker ID whose session context to delete (required)")
	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation when the worker has active tasks")
	cmd.MarkFlagRequired("session-key")
	cmd.MarkFlagRequired("worker-id")
	return cmd
}
