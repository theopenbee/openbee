package daemoncmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

// NewStatusCommand returns the "status" cobra command. exitCode is invoked to
// signal a non-zero process exit when the daemon is not running.
func NewStatusCommand(exitCode ExitCodeFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: i18n.M.Cmd.Status.Short,
		RunE: func(cmd *cobra.Command, args []string) error {
			pidFile, err := config.DaemonPIDFile()
			if err != nil {
				return err
			}
			running, msg := daemonStatus(pidFile)
			fmt.Println(msg)
			if !running {
				return exitCode(1)
			}
			return nil
		},
	}
}

// daemonStatus returns whether the daemon is running and a human-readable status string.
// pidFilePath is injected to allow testing without touching the real PID file.
func daemonStatus(pidFilePath string) (running bool, msg string) {
	pid, startTS, err := readPIDFileFrom(pidFilePath)
	if err != nil {
		return false, i18n.M.Output.Status.NotRunning
	}

	if !utils.IsProcessAlive(pid) {
		_ = os.Remove(pidFilePath) // clean up stale file
		return false, i18n.M.Output.Status.NotRunningStale
	}

	uptime := time.Now().Unix() - startTS
	return true, fmt.Sprintf(i18n.M.Output.Status.Running, pid, formatUptime(uptime))
}
