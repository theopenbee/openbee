package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of the OpenBee daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		running, msg := daemonStatus(daemonPIDFile())
		fmt.Println(msg)
		if !running {
			return &exitCodeError{code: 1}
		}
		return nil
	},
}

// daemonStatus returns whether the daemon is running and a human-readable status string.
// pidFilePath is injected to allow testing without touching the real PID file.
func daemonStatus(pidFilePath string) (running bool, msg string) {
	pid, startTS, err := readPIDFileFrom(pidFilePath)
	if err != nil {
		return false, i18n.M.Output.Status.NotRunning
	}

	if !isAlive(pid) {
		_ = os.Remove(pidFilePath) // clean up stale file
		return false, i18n.M.Output.Status.NotRunningStale
	}

	uptime := time.Now().Unix() - startTS
	return true, fmt.Sprintf(i18n.M.Output.Status.Running, pid, formatUptime(uptime))
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
