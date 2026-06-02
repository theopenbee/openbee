package daemoncmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

// NewStopCommand returns the "stop" cobra command.
func NewStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: i18n.M.Cmd.Stop.Short,
		RunE: func(cmd *cobra.Command, args []string) error {
			pidFile, err := config.DaemonPIDFile()
			if err != nil {
				return err
			}
			return DoStop(pidFile)
		},
	}
}

// DoStop is the testable core of the stop command. pidFile is injected for hermetic testing.
func DoStop(pidFile string) error {
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
