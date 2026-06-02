package ctlcmd

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

func newSessionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: i18n.M.Cmd.CtlSession.Short,
	}

	var sessionListKey string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all agents with active session contexts for a session key",
		RunE: func(c *cobra.Command, args []string) error {
			return ctlRun(utils.ListSessionContexts, map[string]any{"session_key": sessionListKey})
		},
	}
	listCmd.Flags().StringVar(&sessionListKey, "session-key", "", "Session key to query (required)")
	listCmd.MarkFlagRequired("session-key")

	var (
		sessionClearKey   string
		sessionClearForce bool
	)
	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Cancel all active tasks and clear all session contexts for a session key",
		RunE: func(c *cobra.Command, args []string) error {
			return ctlRun(utils.ClearSession, map[string]any{
				"session_key": sessionClearKey,
				"force":       sessionClearForce,
			})
		},
	}
	clearCmd.Flags().StringVar(&sessionClearKey, "session-key", "", "Session key to clear (required)")
	clearCmd.Flags().BoolVar(&sessionClearForce, "force", false, "Skip confirmation when multiple workers are linked")
	clearCmd.MarkFlagRequired("session-key")

	var (
		sessionClearWorkerKey   string
		sessionClearWorkerID    string
		sessionClearWorkerForce bool
	)
	clearWorkerCmd := &cobra.Command{
		Use:   "clear-worker",
		Short: "Cancel one worker's active tasks and reset its session context within a session",
		RunE: func(c *cobra.Command, args []string) error {
			return ctlRun(utils.ClearWorkerSession, map[string]any{
				"session_key": sessionClearWorkerKey,
				"worker_id":   sessionClearWorkerID,
				"force":       sessionClearWorkerForce,
			})
		},
	}
	clearWorkerCmd.Flags().StringVar(&sessionClearWorkerKey, "session-key", "", "Session key (required)")
	clearWorkerCmd.Flags().StringVar(&sessionClearWorkerID, "worker-id", "", "Worker ID whose session context to delete (required)")
	clearWorkerCmd.Flags().BoolVar(&sessionClearWorkerForce, "force", false, "Skip confirmation when the worker has active tasks")
	clearWorkerCmd.MarkFlagRequired("session-key")
	clearWorkerCmd.MarkFlagRequired("worker-id")

	cmd.AddCommand(listCmd, clearCmd, clearWorkerCmd)
	return cmd
}
