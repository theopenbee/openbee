package ctlcmd

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

func newConstraintCommand(run Runner) *cobra.Command {
	subs := i18n.M.Cmd.CtlConstraint
	cmd := &cobra.Command{
		Use:   "constraint",
		Short: subs.Short,
	}
	cmd.AddCommand(
		newConstraintGetCommand(run, subs.Sub("get")),
		newConstraintSaveCommand(run, subs.Sub("save")),
		newConstraintDeleteCommand(run, subs.Sub("delete")),
	)
	return cmd
}

func newConstraintGetCommand(run Runner, short string) *cobra.Command {
	var (
		scope string
		key   string
	)
	cmd := &cobra.Command{
		Use:   "get",
		Short: short,
		RunE: func(c *cobra.Command, args []string) error {
			a := map[string]any{"scope": scope}
			setIfNonEmpty(a, "key", key)
			return run(utils.GetConstraint, a)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "Constraint scope: 'global' or session_key (required)")
	cmd.Flags().StringVar(&key, "key", "", "Constraint key (omit to list all in scope)")
	cmd.MarkFlagRequired("scope")
	return cmd
}

func newConstraintSaveCommand(run Runner, short string) *cobra.Command {
	var (
		scope string
		key   string
		value string
	)
	cmd := &cobra.Command{
		Use:   "save",
		Short: short,
		RunE: func(c *cobra.Command, args []string) error {
			return run(utils.SaveConstraint, map[string]any{
				"scope": scope,
				"key":   key,
				"value": value,
			})
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "Constraint scope: 'global' or session_key (required)")
	cmd.Flags().StringVar(&key, "key", "", "Constraint key (required)")
	cmd.Flags().StringVar(&value, "value", "", "Constraint value (required)")
	cmd.MarkFlagRequired("scope")
	cmd.MarkFlagRequired("key")
	cmd.MarkFlagRequired("value")
	return cmd
}

func newConstraintDeleteCommand(run Runner, short string) *cobra.Command {
	var (
		scope string
		key   string
	)
	cmd := &cobra.Command{
		Use:   "delete",
		Short: short,
		RunE: func(c *cobra.Command, args []string) error {
			return run(utils.DeleteConstraint, map[string]any{
				"scope": scope,
				"key":   key,
			})
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "Constraint scope: 'global' or session_key (required)")
	cmd.Flags().StringVar(&key, "key", "", "Constraint key (required)")
	cmd.MarkFlagRequired("scope")
	cmd.MarkFlagRequired("key")
	return cmd
}
