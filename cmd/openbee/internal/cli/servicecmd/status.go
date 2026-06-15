package servicecmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: i18n.M.Cmd.Service.StatusS,
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
			fmt.Fprintf(out, i18n.M.Output.Service.StatusInstalled+"\n", boolYesNo(st.Installed))
			fmt.Fprintf(out, i18n.M.Output.Service.StatusRunState+"\n", st.RunState.String())
			if st.RunState == RunStateRunning && st.PID > 0 {
				fmt.Fprintf(out, i18n.M.Output.Service.StatusPID+"\n", st.PID)
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
		return i18n.M.Output.Service.StatusYes
	}
	return i18n.M.Output.Service.StatusNo
}
