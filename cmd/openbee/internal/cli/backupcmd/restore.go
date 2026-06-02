package backupcmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/daemoncmd"
	"github.com/theopenbee/openbee/internal/infra/backup"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

var (
	restorePassword string
	restoreForce    bool
)

// NewRestoreCommand constructs the `restore` command. appVersion is the build-time
// version string used to validate backup compatibility.
func NewRestoreCommand(appVersion string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore <backup-file>",
		Short: i18n.M.Cmd.Restore.Short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRestore(args, appVersion)
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", i18n.M.Flag.ConfigPath)
	cmd.Flags().StringVar(&restorePassword, "password", "", i18n.M.Flag.RestorePassword)
	cmd.Flags().BoolVar(&restoreForce, "force", false, i18n.M.Flag.RestoreForce)
	return cmd
}

func runRestore(args []string, appVersion string) error {
	archivePath := args[0]

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Stop daemon if running before overwriting data.
	if err := daemoncmd.DoStop(daemoncmd.DaemonPIDFile()); err != nil {
		return fmt.Errorf("stop daemon before restore: %w", err)
	}

	if err := backup.Restore(backup.RestoreOptions{
		ArchivePath: archivePath,
		DBPath:      cfg.Database.Path,
		ConfigPath:  cfgPath,
		StateDir:    daemoncmd.OpenbeeStateDir(),
		AppVersion:  appVersion,
		Force:       restoreForce,
		Password:    restorePassword,
	}); err != nil {
		return err
	}

	fmt.Println(i18n.M.Output.Restore.Complete)
	return nil
}
