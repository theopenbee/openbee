package backupcmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/daemoncmd"
	"github.com/theopenbee/openbee/internal/infra/backup"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func NewRestoreCommand(appVersion string) *cobra.Command {
	var (
		cfgPath         string
		restorePassword string
		restoreForce    bool
	)
	cmd := &cobra.Command{
		Use:   "restore <backup-file>",
		Short: i18n.M.Cmd.Restore.Short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRestore(args, appVersion, cfgPath, restorePassword, restoreForce)
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", i18n.M.Flag.ConfigPath)
	cmd.Flags().StringVar(&restorePassword, "password", "", i18n.M.Flag.RestorePassword)
	cmd.Flags().BoolVar(&restoreForce, "force", false, i18n.M.Flag.RestoreForce)
	return cmd
}

func runRestore(args []string, appVersion, cfgPath, password string, force bool) error {
	archivePath := args[0]

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	stateDir, err := config.OpenbeeHomeDir()
	if err != nil {
		return err
	}
	pidFile, err := config.DaemonPIDFile()
	if err != nil {
		return err
	}

	// Stop daemon if running before overwriting data.
	if err := daemoncmd.DoStop(pidFile); err != nil {
		return fmt.Errorf("stop daemon before restore: %w", err)
	}

	if err := backup.Restore(backup.RestoreOptions{
		ArchivePath: archivePath,
		DBPath:      cfg.Database.Path,
		ConfigPath:  cfgPath,
		StateDir:    stateDir,
		AppVersion:  appVersion,
		Force:       force,
		Password:    password,
	}); err != nil {
		return err
	}

	fmt.Println(i18n.M.Output.Restore.Complete)
	return nil
}
