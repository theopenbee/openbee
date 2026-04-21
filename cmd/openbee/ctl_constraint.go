package main

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

var ctlConstraintCmd = &cobra.Command{Use: "constraint", Short: ""}

var (
	constraintGetScope string
	constraintGetKey   string
)

var ctlConstraintGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Read constraint entries (omit --key to list all in scope)",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"scope": constraintGetScope}
		if constraintGetKey != "" {
			a["key"] = constraintGetKey
		}
		return ctlRun(utils.GetConstraint, a)
	},
}

var (
	constraintSaveScope string
	constraintSaveKey   string
	constraintSaveValue string
)

var ctlConstraintSaveCmd = &cobra.Command{
	Use:   "save",
	Short: "Save or update a constraint entry",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.SaveConstraint, map[string]any{
			"scope": constraintSaveScope,
			"key":   constraintSaveKey,
			"value": constraintSaveValue,
		})
	},
}

var (
	constraintDeleteScope string
	constraintDeleteKey   string
)

var ctlConstraintDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a constraint entry",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.DeleteConstraint, map[string]any{
			"scope": constraintDeleteScope,
			"key":   constraintDeleteKey,
		})
	},
}

func init() {
	ctlConstraintGetCmd.Flags().StringVar(&constraintGetScope, "scope", "", "Constraint scope: 'global' or session_key (required)")
	ctlConstraintGetCmd.Flags().StringVar(&constraintGetKey, "key", "", "Constraint key (omit to list all in scope)")
	ctlConstraintGetCmd.MarkFlagRequired("scope")

	ctlConstraintSaveCmd.Flags().StringVar(&constraintSaveScope, "scope", "", "Constraint scope: 'global' or session_key (required)")
	ctlConstraintSaveCmd.Flags().StringVar(&constraintSaveKey, "key", "", "Constraint key (required)")
	ctlConstraintSaveCmd.Flags().StringVar(&constraintSaveValue, "value", "", "Constraint value (required)")
	ctlConstraintSaveCmd.MarkFlagRequired("scope")
	ctlConstraintSaveCmd.MarkFlagRequired("key")
	ctlConstraintSaveCmd.MarkFlagRequired("value")

	ctlConstraintDeleteCmd.Flags().StringVar(&constraintDeleteScope, "scope", "", "Constraint scope: 'global' or session_key (required)")
	ctlConstraintDeleteCmd.Flags().StringVar(&constraintDeleteKey, "key", "", "Constraint key (required)")
	ctlConstraintDeleteCmd.MarkFlagRequired("scope")
	ctlConstraintDeleteCmd.MarkFlagRequired("key")

	ctlConstraintCmd.AddCommand(ctlConstraintGetCmd, ctlConstraintSaveCmd, ctlConstraintDeleteCmd)
	ctlCmd.AddCommand(ctlConstraintCmd)
}
