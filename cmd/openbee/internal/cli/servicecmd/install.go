package servicecmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func newInstallCommand() *cobra.Command {
	var (
		configFlag string
		noStart    bool
		force      bool
	)
	cmd := &cobra.Command{
		Use:   "install",
		Short: i18n.M.Cmd.Service.Install,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := resolveInstallOptions(configFlag, noStart, force)
			if err != nil {
				return err
			}
			mgr, err := newManager()
			if err != nil {
				return err
			}
			if err := mgr.Install(cmd.Context(), opts); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), i18n.M.Output.Service.Installed+"\n", opts.ConfigPath)
			if !opts.AutoStart {
				return nil
			}
			return reportRunStateAfterStart(cmd.Context(), mgr, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&configFlag, "config", "", i18n.M.Flag.ServiceConfig)
	cmd.Flags().BoolVar(&noStart, "no-start", false, i18n.M.Flag.ServiceNoStart)
	cmd.Flags().BoolVar(&force, "force", false, i18n.M.Flag.ServiceForce)
	return cmd
}
