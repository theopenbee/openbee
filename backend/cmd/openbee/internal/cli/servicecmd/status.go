package servicecmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/config"
)

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show autostart installation and run state",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}
			st, err := mgr.Status(cmd.Context())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Installed: %s\n", boolYesNo(st.Installed))
			fmt.Fprintf(out, "State:     %s\n", st.RunState.String())
			if st.RunState == RunStateRunning && st.PID > 0 {
				fmt.Fprintf(out, "PID:       %d\n", st.PID)
			}
			if st.Installed && st.RunState != RunStateRunning {
				logPath, _ := config.DaemonLogFile()
				printStartFailureDetails(out, st, logPath)
			}
			return nil
		},
	}
}

func boolYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
