package main

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/toolnames"
)

var ctlMemoryCmd = &cobra.Command{Use: "memory", Short: ""}

var (
	memoryGetScope string
	memoryGetKey   string
)

var ctlMemoryGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Read memory entries (omit --key to list all in scope)",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"scope": memoryGetScope}
		if memoryGetKey != "" {
			a["key"] = memoryGetKey
		}
		return ctlRun(toolnames.GetMemory, a)
	},
}

var (
	memorySaveScope string
	memorySaveKey   string
	memorySaveValue string
)

var ctlMemorySaveCmd = &cobra.Command{
	Use:   "save",
	Short: "Save or update a memory entry",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(toolnames.SaveMemory, map[string]any{
			"scope": memorySaveScope,
			"key":   memorySaveKey,
			"value": memorySaveValue,
		})
	},
}

var (
	memoryDeleteScope string
	memoryDeleteKey   string
)

var ctlMemoryDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a memory entry",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(toolnames.DeleteMemory, map[string]any{
			"scope": memoryDeleteScope,
			"key":   memoryDeleteKey,
		})
	},
}

func init() {
	ctlMemoryGetCmd.Flags().StringVar(&memoryGetScope, "scope", "", "Memory scope: 'global' or session_key (required)")
	ctlMemoryGetCmd.Flags().StringVar(&memoryGetKey, "key", "", "Memory key (omit to list all in scope)")
	ctlMemoryGetCmd.MarkFlagRequired("scope")

	ctlMemorySaveCmd.Flags().StringVar(&memorySaveScope, "scope", "", "Memory scope: 'global' or session_key (required)")
	ctlMemorySaveCmd.Flags().StringVar(&memorySaveKey, "key", "", "Memory key (required)")
	ctlMemorySaveCmd.Flags().StringVar(&memorySaveValue, "value", "", "Memory value (required)")
	ctlMemorySaveCmd.MarkFlagRequired("scope")
	ctlMemorySaveCmd.MarkFlagRequired("key")
	ctlMemorySaveCmd.MarkFlagRequired("value")

	ctlMemoryDeleteCmd.Flags().StringVar(&memoryDeleteScope, "scope", "", "Memory scope: 'global' or session_key (required)")
	ctlMemoryDeleteCmd.Flags().StringVar(&memoryDeleteKey, "key", "", "Memory key (required)")
	ctlMemoryDeleteCmd.MarkFlagRequired("scope")
	ctlMemoryDeleteCmd.MarkFlagRequired("key")

	ctlMemoryCmd.AddCommand(ctlMemoryGetCmd, ctlMemorySaveCmd, ctlMemoryDeleteCmd)
	ctlCmd.AddCommand(ctlMemoryCmd)
}
