package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of the OpenBee daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		running, msg := daemonStatus(daemonPIDFile())
		fmt.Println(msg)
		if !running {
			os.Exit(1)
		}
		return nil
	},
}

// daemonStatus returns whether the daemon is running and a human-readable status string.
// pidFilePath is injected to allow testing without touching the real PID file.
func daemonStatus(pidFilePath string) (running bool, msg string) {
	pid, startTS, err := readPIDFileFrom(pidFilePath)
	if err != nil {
		return false, "○ openbee is not running"
	}

	if !isAlive(pid) {
		_ = os.Remove(pidFilePath) // clean up stale file
		return false, "○ openbee is not running (stale PID file removed)"
	}

	uptime := time.Now().Unix() - startTS
	return true, fmt.Sprintf("● openbee is running   (PID: %d, uptime: %s)", pid, formatUptime(uptime))
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
