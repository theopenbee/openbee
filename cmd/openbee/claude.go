package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	claude "github.com/theopenbee/openbee/internal/ai/engine/claude"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

var claudeCmd = &cobra.Command{
	Use:   "claude",
	Short: "Manage Claude Code installation and provider configuration",
}

var (
	claudeDownloadForce  bool
	claudeDownloadCDNURL string
	claudeDownloadCN     bool
)

var claudeDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download Claude Code binary to ~/.openbee/bin/claude",
	RunE: func(cmd *cobra.Command, args []string) error {
		cdnURL := resolveCDNURL(claudeDownloadCDNURL, claudeDownloadCN)
		stateDir := openbeeStateDir()
		destPath := filepath.Join(stateDir, "bin", "claude")
		if !claudeDownloadForce {
			if _, err := os.Stat(destPath); err == nil {
				fmt.Printf(i18n.M.Output.Claude.AlreadyInstalled+"\n", destPath)
				fmt.Println(i18n.M.Output.Claude.UseForce)
				return nil
			}
		}
		if cdnURL != "" {
			fmt.Printf(i18n.M.Output.Claude.UsingCDN+"\n", cdnURL)
		}
		path, err := claude.Download(stateDir, claudeDownloadForce, cdnURL)
		if err != nil {
			return err
		}
		fmt.Printf(i18n.M.Output.Claude.InstalledAt+"\n", path)
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
	claudeDownloadCmd.Flags().BoolVar(&claudeDownloadForce, "force", false, i18n.M.Flag.ClaudeDownloadForce)
	claudeDownloadCmd.Flags().StringVar(&claudeDownloadCDNURL, "cdn-url", "", i18n.M.Flag.ClaudeDownloadCDNURL)
	claudeDownloadCmd.Flags().BoolVar(&claudeDownloadCN, "cn", false, i18n.M.Flag.ClaudeDownloadCN)
	claudeCmd.AddCommand(claudeDownloadCmd)
	claudeCmd.AddCommand(claudeEnvCmd)
	rootCmd.AddCommand(claudeCmd)
}
