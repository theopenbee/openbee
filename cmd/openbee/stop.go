package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
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
		fmt.Println(i18n.M.Output.Stop.NotRunning)
		return nil
	}

	if !utils.IsProcessAlive(pid) {
		if isPIDForeign(pid) {
			fmt.Fprintf(os.Stderr, i18n.M.Output.Stop.ForeignPID+"\n", pid)
		} else {
			fmt.Println(i18n.M.Output.Stop.Stale)
		}
		return os.Remove(pidFile)
	}

	fmt.Printf(i18n.M.Output.Stop.Stopping+"\n", pid)
	if err := stopProcess(pid); err != nil {
		return fmt.Errorf("stop process: %w", err)
	}

	// Daemon removes PID file on clean exit; remove it here if still present.
	_ = os.Remove(pidFile)
	fmt.Println(i18n.M.Output.Stop.Stopped)
	return nil
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
