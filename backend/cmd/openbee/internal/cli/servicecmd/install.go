package servicecmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newInstallCommand() *cobra.Command {
	var (
		configFlag     string
		workingDirFlag string
		runAsFlag      string
		noStart        bool
		force          bool
	)
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install autostart and (by default) start the service immediately",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, warnings, err := resolveInstallOptions(configFlag, workingDirFlag, runAsFlag, noStart, force)
			if err != nil {
				return err
			}
			for _, w := range warnings {
				fmt.Fprintln(cmd.ErrOrStderr(), w)
			}
			mgr, err := newManager()
			if err != nil {
				return err
			}
			if err := mgr.Install(cmd.Context(), opts); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "service installed at %s\n", opts.ConfigPath)
			if !opts.AutoStart {
				return nil
			}
			return reportRunStateAfterStart(cmd.Context(), mgr, cmd.OutOrStdout(), opts.LogPath)
		},
	}
	cmd.Flags().StringVar(&configFlag, "config", "", "Path to openbee config file (defaults to ~/.openbee/config.yaml)")
	cmd.Flags().StringVar(&workingDirFlag, "working-dir", "", "Working directory the service runs in (defaults to ~/.openbee)")
	cmd.Flags().StringVar(&runAsFlag, "run-as", "", "Linux: user the systemd service runs as (defaults to $SUDO_USER)")
	cmd.Flags().BoolVar(&noStart, "no-start", false, "Only register autostart, do not start the service")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing installation")
	return cmd
}
