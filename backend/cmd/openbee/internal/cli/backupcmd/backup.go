package backupcmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/backup"
	"github.com/theopenbee/openbee/internal/infra/config"
)

func NewBackupCommand(appVersion string) *cobra.Command {
	var (
		cfgPath        string
		backupPassword string
	)
	cmd := &cobra.Command{
		Use:   "backup [output-dir]",
		Short: "Create a backup archive of the openbee database, config, and state",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackup(args, appVersion, cfgPath, backupPassword)
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "path to config file")
	cmd.Flags().StringVar(&backupPassword, "password", "", "encrypt the backup with this password")
	return cmd
}

func runBackup(args []string, appVersion, cfgPath, password string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	stateDir, err := config.OpenbeeHomeDir()
	if err != nil {
		return err
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
		StateDir:   stateDir,
		OutputDir:  outputDir,
		AppVersion: appVersion,
		Password:   password,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Backup created: %s\n", archivePath)
	return nil
}
