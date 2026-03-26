package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	claude "github.com/theopenbee/openbee/internal/claude"
)

var claudeCmd = &cobra.Command{
	Use:   "claude",
	Short: "Manage Claude Code installation and provider configuration",
}

var claudeDownloadForce bool

var claudeDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download Claude Code binary to ~/.openbee/bin/claude",
	RunE: func(cmd *cobra.Command, args []string) error {
		stateDir := openbeeStateDir()
		destPath := filepath.Join(stateDir, "bin", "claude")
		if !claudeDownloadForce {
			if _, err := os.Stat(destPath); err == nil {
				fmt.Printf("Claude is already installed at %s\n", destPath)
				fmt.Println("Use --force to re-download.")
				return nil
			}
		}
		path, err := claude.Download(stateDir, claudeDownloadForce)
		if err != nil {
			return err
		}
		fmt.Printf("Claude installed at: %s\n", path)
		return nil
	},
}

var claudeEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Configure Claude Code provider and environment settings",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := claude.ConfigureProvider(); err != nil {
			if errors.Is(err, claude.ErrInterrupted) {
				return nil
			}
			return err
		}
		return nil
	},
}

func init() {
	claudeDownloadCmd.Flags().BoolVar(&claudeDownloadForce, "force", false, "Force re-download even if already installed")
	claudeCmd.AddCommand(claudeDownloadCmd)
	claudeCmd.AddCommand(claudeEnvCmd)
	rootCmd.AddCommand(claudeCmd)
}
