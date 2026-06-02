package ctlcmd

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

func newDepartmentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "department",
		Short: "Manage departments",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all departments (tree structure)",
		RunE: func(c *cobra.Command, args []string) error {
			return ctlRun(utils.ListDepartments, nil)
		},
	}

	getCmd := &cobra.Command{
		Use:   "get <id|name>",
		Short: "Get a department by ID or name",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return ctlRun(utils.GetDepartment, map[string]any{"id": args[0]})
		},
	}

	var (
		departmentCreateName      string
		departmentCreateParent    string
		departmentCreateSortOrder int
	)
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new department",
		RunE: func(c *cobra.Command, args []string) error {
			a := map[string]any{"name": departmentCreateName}
			if departmentCreateParent != "" {
				a["parent_id"] = departmentCreateParent
			}
			if c.Flags().Changed("sort-order") {
				a["sort_order"] = departmentCreateSortOrder
			}
			return ctlRun(utils.CreateDepartment, a)
		},
	}
	createCmd.Flags().StringVarP(&departmentCreateName, "name", "n", "", "Department name (required)")
	createCmd.MarkFlagRequired("name")
	createCmd.Flags().StringVar(&departmentCreateParent, "parent", "", "Parent department ID or name")
	createCmd.Flags().IntVar(&departmentCreateSortOrder, "sort-order", 0, "Display sort order")

	var (
		departmentUpdateName      string
		departmentUpdateParent    string
		departmentUpdateSortOrder int
	)
	updateCmd := &cobra.Command{
		Use:   "update <id|name>",
		Short: "Update a department (patch: omitted fields unchanged)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			a := map[string]any{"id": args[0]}
			if c.Flags().Changed("name") {
				a["name"] = departmentUpdateName
			}
			if c.Flags().Changed("parent") {
				a["parent_id"] = departmentUpdateParent
			}
			if c.Flags().Changed("sort-order") {
				a["sort_order"] = departmentUpdateSortOrder
			}
			return ctlRun(utils.UpdateDepartment, a)
		},
	}
	updateCmd.Flags().StringVar(&departmentUpdateName, "name", "", "New name")
	updateCmd.Flags().StringVar(&departmentUpdateParent, "parent", "", "New parent department ID or name")
	updateCmd.Flags().IntVar(&departmentUpdateSortOrder, "sort-order", 0, "New sort order")

	deleteCmd := &cobra.Command{
		Use:   "delete <id|name>",
		Short: "Delete a department",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return ctlRun(utils.DeleteDepartment, map[string]any{"id": args[0]})
		},
	}

	cmd.AddCommand(listCmd, getCmd, createCmd, updateCmd, deleteCmd)
	return cmd
}
