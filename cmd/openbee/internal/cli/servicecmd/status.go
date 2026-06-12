package servicecmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

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
				fmt.Fprintf(out, i18n.M.Output.Service.StatusPIDUptime+"\n", st.PID, formatUptime(time.Duration(st.UptimeSecs)*time.Second))
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

func formatUptime(d time.Duration) string {
	secs := int64(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	if secs < 3600 {
		return fmt.Sprintf("%dm %ds", secs/60, secs%60)
	}
	return fmt.Sprintf("%dh %dm", secs/3600, (secs%3600)/60)
}
