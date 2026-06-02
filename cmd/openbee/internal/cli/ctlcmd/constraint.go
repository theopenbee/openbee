package ctlcmd

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

func newConstraintCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "constraint",
		Short: i18n.M.Cmd.CtlConstraint.Short,
	}

	var (
		constraintGetScope string
		constraintGetKey   string
	)
	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Read constraint entries (omit --key to list all in scope)",
		RunE: func(c *cobra.Command, args []string) error {
			a := map[string]any{"scope": constraintGetScope}
			if constraintGetKey != "" {
				a["key"] = constraintGetKey
			}
			return ctlRun(utils.GetConstraint, a)
		},
	}
	getCmd.Flags().StringVar(&constraintGetScope, "scope", "", "Constraint scope: 'global' or session_key (required)")
	getCmd.Flags().StringVar(&constraintGetKey, "key", "", "Constraint key (omit to list all in scope)")
	getCmd.MarkFlagRequired("scope")

	var (
		constraintSaveScope string
		constraintSaveKey   string
		constraintSaveValue string
	)
	saveCmd := &cobra.Command{
		Use:   "save",
		Short: "Save or update a constraint entry",
		RunE: func(c *cobra.Command, args []string) error {
			return ctlRun(utils.SaveConstraint, map[string]any{
				"scope": constraintSaveScope,
				"key":   constraintSaveKey,
				"value": constraintSaveValue,
			})
		},
	}
	saveCmd.Flags().StringVar(&constraintSaveScope, "scope", "", "Constraint scope: 'global' or session_key (required)")
	saveCmd.Flags().StringVar(&constraintSaveKey, "key", "", "Constraint key (required)")
	saveCmd.Flags().StringVar(&constraintSaveValue, "value", "", "Constraint value (required)")
	saveCmd.MarkFlagRequired("scope")
	saveCmd.MarkFlagRequired("key")
	saveCmd.MarkFlagRequired("value")

	var (
		constraintDeleteScope string
		constraintDeleteKey   string
	)
	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a constraint entry",
		RunE: func(c *cobra.Command, args []string) error {
			return ctlRun(utils.DeleteConstraint, map[string]any{
				"scope": constraintDeleteScope,
				"key":   constraintDeleteKey,
			})
		},
	}
	deleteCmd.Flags().StringVar(&constraintDeleteScope, "scope", "", "Constraint scope: 'global' or session_key (required)")
	deleteCmd.Flags().StringVar(&constraintDeleteKey, "key", "", "Constraint key (required)")
	deleteCmd.MarkFlagRequired("scope")
	deleteCmd.MarkFlagRequired("key")

	cmd.AddCommand(getCmd, saveCmd, deleteCmd)
	return cmd
}
