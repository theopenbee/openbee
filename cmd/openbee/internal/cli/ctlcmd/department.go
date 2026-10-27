package ctlcmd

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

func newDepartmentCommand(run Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "department",
		Short: "Manage departments",
	}
	cmd.AddCommand(
		newDepartmentListCommand(run, "List all departments (tree structure)"),
		newDepartmentGetCommand(run, "Get a department by ID or name"),
		newDepartmentCreateCommand(run, "Create a new department"),
		newDepartmentUpdateCommand(run, "Update a department (patch: omitted fields unchanged)"),
		newDepartmentDeleteCommand(run, "Delete a department"),
	)
	return cmd
}

func newDepartmentListCommand(run Runner, short string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: short,
		RunE: func(c *cobra.Command, args []string) error {
			return run(utils.ListDepartments, nil)
		},
	}
}

func newDepartmentGetCommand(run Runner, short string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id|name>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return run(utils.GetDepartment, map[string]any{"id": args[0]})
		},
	}
}

func newDepartmentCreateCommand(run Runner, short string) *cobra.Command {
	var (
		name      string
		parent    string
		sortOrder int
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: short,
		RunE: func(c *cobra.Command, args []string) error {
			a := map[string]any{"name": name}
			setIfNonEmpty(a, "parent_id", parent)
			setIfFlagChanged(c, a, "sort-order", "sort_order", sortOrder)
			return run(utils.CreateDepartment, a)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "", "Department name (required)")
	cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&parent, "parent", "", "Parent department ID or name")
	cmd.Flags().IntVar(&sortOrder, "sort-order", 0, "Display sort order")
	return cmd
}

func newDepartmentUpdateCommand(run Runner, short string) *cobra.Command {
	var (
		name      string
		parent    string
		sortOrder int
	)
	cmd := &cobra.Command{
		Use:   "update <id|name>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			a := map[string]any{"id": args[0]}
			setIfFlagChanged(c, a, "name", "name", name)
			setIfFlagChanged(c, a, "parent", "parent_id", parent)
			setIfFlagChanged(c, a, "sort-order", "sort_order", sortOrder)
			return run(utils.UpdateDepartment, a)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "New name")
	cmd.Flags().StringVar(&parent, "parent", "", "New parent department ID or name")
	cmd.Flags().IntVar(&sortOrder, "sort-order", 0, "New sort order")
	return cmd
}

func newDepartmentDeleteCommand(run Runner, short string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id|name>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return run(utils.DeleteDepartment, map[string]any{"id": args[0]})
		},
	}
}
