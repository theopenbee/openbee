package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/backup"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

var restorePassword string
var restoreForce bool

var restoreCmd = &cobra.Command{
	Use:   "restore <backup-file>",
	Short: "Restore openbee data from a backup archive",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		archivePath := args[0]

		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		// Stop daemon if running before overwriting data.
		if err := doStop(daemonPIDFile()); err != nil {
			return fmt.Errorf("stop daemon before restore: %w", err)
		}

		if err := backup.Restore(backup.RestoreOptions{
			ArchivePath: archivePath,
			DBPath:      cfg.Database.Path,
			ConfigPath:  cfgPath,
			StateDir:    openbeeStateDir(),
			AppVersion:  version,
			Force:       restoreForce,
			Password:    restorePassword,
		}); err != nil {
			return err
		}

		fmt.Println(i18n.M.Output.Restore.Complete)
		return nil
	},
}

func init() {
	restoreCmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "path to config file")
	restoreCmd.Flags().StringVar(&restorePassword, "password", "", "password to decrypt the backup archive")
	restoreCmd.Flags().BoolVar(&restoreForce, "force", false, "overwrite existing data")
	rootCmd.AddCommand(restoreCmd)
}
