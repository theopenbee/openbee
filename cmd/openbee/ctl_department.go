package main

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

var ctlDepartmentCmd = &cobra.Command{Use: "department", Short: "Manage departments"}

var ctlDepartmentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all departments (tree structure)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.ListDepartments, nil)
	},
}

var ctlDepartmentGetCmd = &cobra.Command{
	Use:   "get <id|name>",
	Short: "Get a department by ID or name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.GetDepartment, map[string]any{"id": args[0]})
	},
}

var (
	departmentCreateName      string
	departmentCreateParent    string
	departmentCreateSortOrder int
)

var ctlDepartmentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new department",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"name": departmentCreateName}
		if departmentCreateParent != "" {
			a["parent_id"] = departmentCreateParent
		}
		if cmd.Flags().Changed("sort-order") {
			a["sort_order"] = departmentCreateSortOrder
		}
		return ctlRun(utils.CreateDepartment, a)
	},
}

var (
	departmentUpdateName      string
	departmentUpdateParent    string
	departmentUpdateSortOrder int
)

var ctlDepartmentUpdateCmd = &cobra.Command{
	Use:   "update <id|name>",
	Short: "Update a department (patch: omitted fields unchanged)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"id": args[0]}
		if cmd.Flags().Changed("name") {
			a["name"] = departmentUpdateName
		}
		if cmd.Flags().Changed("parent") {
			a["parent_id"] = departmentUpdateParent
		}
		if cmd.Flags().Changed("sort-order") {
			a["sort_order"] = departmentUpdateSortOrder
		}
		return ctlRun(utils.UpdateDepartment, a)
	},
}

var ctlDepartmentDeleteCmd = &cobra.Command{
	Use:   "delete <id|name>",
	Short: "Delete a department",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.DeleteDepartment, map[string]any{"id": args[0]})
	},
}

func init() {
	ctlDepartmentCreateCmd.Flags().StringVarP(&departmentCreateName, "name", "n", "", "Department name (required)")
	ctlDepartmentCreateCmd.MarkFlagRequired("name")
	ctlDepartmentCreateCmd.Flags().StringVar(&departmentCreateParent, "parent", "", "Parent department ID or name")
	ctlDepartmentCreateCmd.Flags().IntVar(&departmentCreateSortOrder, "sort-order", 0, "Display sort order")

	ctlDepartmentUpdateCmd.Flags().StringVar(&departmentUpdateName, "name", "", "New name")
	ctlDepartmentUpdateCmd.Flags().StringVar(&departmentUpdateParent, "parent", "", "New parent department ID or name")
	ctlDepartmentUpdateCmd.Flags().IntVar(&departmentUpdateSortOrder, "sort-order", 0, "New sort order")

	ctlDepartmentCmd.AddCommand(
		ctlDepartmentListCmd,
		ctlDepartmentGetCmd,
		ctlDepartmentCreateCmd,
		ctlDepartmentUpdateCmd,
		ctlDepartmentDeleteCmd,
	)
	ctlCmd.AddCommand(ctlDepartmentCmd)
}
