package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/backup"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

var backupPassword string

var backupCmd = &cobra.Command{
	Use:   "backup [output-dir]",
	Short: "Create a backup archive of the openbee database, config, and state",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		outputDir := "."
		if len(args) == 1 {
			outputDir = args[0]
		}
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}

		archivePath, err := backup.Backup(backup.BackupOptions{
			DBPath:     cfg.Database.Path,
			ConfigPath: cfgPath,
			StateDir:   openbeeStateDir(),
			OutputDir:  outputDir,
			AppVersion: version,
			Password:   backupPassword,
		})
		if err != nil {
			return err
		}
		fmt.Printf(i18n.M.Output.Backup.Created+"\n", archivePath)
		return nil
	},
}

func init() {
	backupCmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "path to config file")
	backupCmd.Flags().StringVar(&backupPassword, "password", "", "encrypt the backup with this password")
	rootCmd.AddCommand(backupCmd)
}
