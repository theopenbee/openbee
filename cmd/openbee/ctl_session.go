package main

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/toolnames"
)

var ctlSessionCmd = &cobra.Command{Use: "session", Short: ""}

var (
	sessionListKey        string
	sessionClearKey       string
	sessionClearForce     bool
	sessionClearWorkerKey string
	sessionClearWorkerID  string
)

var ctlSessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all agents with active session contexts for a session key",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(toolnames.ListSessionContexts, map[string]any{"session_key": sessionListKey})
	},
}

var ctlSessionClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Cancel all active tasks and clear all session contexts for a session key",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(toolnames.ClearSession, map[string]any{
			"session_key": sessionClearKey,
			"force":       sessionClearForce,
		})
	},
}

var ctlSessionClearWorkerCmd = &cobra.Command{
	Use:   "clear-worker",
	Short: "Reset one worker's session context within a session",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(toolnames.ClearWorkerSession, map[string]any{
			"session_key": sessionClearWorkerKey,
			"worker_id":   sessionClearWorkerID,
		})
	},
}

func init() {
	ctlSessionListCmd.Flags().StringVar(&sessionListKey, "session-key", "", "Session key to query (required)")
	ctlSessionListCmd.MarkFlagRequired("session-key")

	ctlSessionClearCmd.Flags().StringVar(&sessionClearKey, "session-key", "", "Session key to clear (required)")
	ctlSessionClearCmd.Flags().BoolVar(&sessionClearForce, "force", false, "Skip confirmation when multiple workers are linked")
	ctlSessionClearCmd.MarkFlagRequired("session-key")

	ctlSessionClearWorkerCmd.Flags().StringVar(&sessionClearWorkerKey, "session-key", "", "Session key (required)")
	ctlSessionClearWorkerCmd.Flags().StringVar(&sessionClearWorkerID, "worker-id", "", "Worker ID whose session context to delete (required)")
	ctlSessionClearWorkerCmd.MarkFlagRequired("session-key")
	ctlSessionClearWorkerCmd.MarkFlagRequired("worker-id")

	ctlSessionCmd.AddCommand(ctlSessionListCmd, ctlSessionClearCmd, ctlSessionClearWorkerCmd)
	ctlCmd.AddCommand(ctlSessionCmd)
}
