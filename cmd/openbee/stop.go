package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running OpenBee daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		return doStop(daemonPIDFile())
	},
}

// doStop is the testable core of stopCmd. pidFile is injected for hermetic testing.
func doStop(pidFile string) error {
	pid, _, err := readPIDFileFrom(pidFile)
	if err != nil {
		// No PID file — daemon is not running.
		fmt.Println("openbee is not running")
		return nil
	}

	if !isAlive(pid) {
		fmt.Println("openbee is not running (stale PID file removed)")
		return os.Remove(pidFile)
	}

	fmt.Printf("Stopping openbee (PID: %d)...\n", pid)
	if err := stopProcess(pid); err != nil {
		return fmt.Errorf("stop process: %w", err)
	}

	// Daemon removes PID file on clean exit; remove it here if still present.
	_ = os.Remove(pidFile)
	fmt.Println("openbee stopped")
	return nil
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
