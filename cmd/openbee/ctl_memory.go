package main

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/toolnames"
)

var ctlMemoryCmd = &cobra.Command{Use: "memory", Short: ""}

var (
	memoryScope string
	memoryKey   string
	memoryValue string
)

var ctlMemoryGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Read memory entries (omit --key to list all in scope)",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"scope": memoryScope}
		if memoryKey != "" {
			a["key"] = memoryKey
		}
		return ctlRun(toolnames.GetMemory, a)
	},
}

var ctlMemorySaveCmd = &cobra.Command{
	Use:   "save",
	Short: "Save or update a memory entry",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(toolnames.SaveMemory, map[string]any{
			"scope": memoryScope,
			"key":   memoryKey,
			"value": memoryValue,
		})
	},
}

var ctlMemoryDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a memory entry",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(toolnames.DeleteMemory, map[string]any{
			"scope": memoryScope,
			"key":   memoryKey,
		})
	},
}

func init() {
	for _, cmd := range []*cobra.Command{ctlMemoryGetCmd, ctlMemorySaveCmd, ctlMemoryDeleteCmd} {
		cmd.Flags().StringVar(&memoryScope, "scope", "", "Memory scope: 'global' or session_key (required)")
		cmd.MarkFlagRequired("scope")
	}

	ctlMemoryGetCmd.Flags().StringVar(&memoryKey, "key", "", "Memory key (omit to list all in scope)")

	ctlMemorySaveCmd.Flags().StringVar(&memoryKey, "key", "", "Memory key (required)")
	ctlMemorySaveCmd.Flags().StringVar(&memoryValue, "value", "", "Memory value (required)")
	ctlMemorySaveCmd.MarkFlagRequired("key")
	ctlMemorySaveCmd.MarkFlagRequired("value")

	ctlMemoryDeleteCmd.Flags().StringVar(&memoryKey, "key", "", "Memory key (required)")
	ctlMemoryDeleteCmd.MarkFlagRequired("key")

	ctlMemoryCmd.AddCommand(ctlMemoryGetCmd, ctlMemorySaveCmd, ctlMemoryDeleteCmd)
	ctlCmd.AddCommand(ctlMemoryCmd)
}
