package main

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

var ctlWorkerCmd = &cobra.Command{Use: "worker", Short: ""}

var ctlWorkerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all workers",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{}
		if workerListDepartment != "" {
			a["department_id"] = workerListDepartment
			if workerListNoRecursive {
				a["recursive"] = false
			}
		}
		return ctlRun(utils.ListWorkers, a)
	},
}

var ctlWorkerGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a worker by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.GetWorker, map[string]any{"worker_id": args[0]})
	},
}

var ctlWorkerStatusCmd = &cobra.Command{
	Use:   "status <id>",
	Short: "Get current status of a worker",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.GetWorkerStatus, map[string]any{"worker_id": args[0]})
	},
}

var (
	workerCreateName        string
	workerCreateDescription string
	workerCreateMemory      string
	workerCreateWorkDir     string
	workerCreateScopes      string
	workerUpdateScopes      string
)

var ctlWorkerCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new worker",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"name": workerCreateName}
		if workerCreateDescription != "" {
			a["description"] = workerCreateDescription
		}
		if workerCreateMemory != "" {
			a["memory"] = workerCreateMemory
		}
		if workerCreateWorkDir != "" {
			a["work_dir"] = workerCreateWorkDir
		}
		if workerCreateDepartment != "" {
			a["department_ids"] = workerCreateDepartment
		}
		if workerCreateScopes != "" {
			a["permission_scopes"] = workerCreateScopes
		}
		return ctlRun(utils.CreateWorker, a)
	},
}

var (
	workerUpdateName        string
	workerUpdateDescription string
	workerUpdateMemory      string
)

var ctlWorkerUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a worker (patch: omitted fields unchanged)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"worker_id": args[0]}
		if cmd.Flags().Changed("name") {
			a["name"] = workerUpdateName
		}
		if cmd.Flags().Changed("description") {
			a["description"] = workerUpdateDescription
		}
		if cmd.Flags().Changed("memory") {
			a["memory"] = workerUpdateMemory
		}
		if cmd.Flags().Changed("department") {
			a["department_ids"] = workerUpdateDepartment
		}
		if cmd.Flags().Changed("scopes") {
			a["permission_scopes"] = workerUpdateScopes
		}
		return ctlRun(utils.UpdateWorker, a)
	},
}

var (
	workerListDepartment   string
	workerListNoRecursive  bool
	workerCreateDepartment string
	workerUpdateDepartment string
)

var workerDeleteWorkDir bool

var ctlWorkerDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a worker",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.DeleteWorker, map[string]any{
			"worker_id":       args[0],
			"delete_work_dir": workerDeleteWorkDir,
		})
	},
}

func init() {
	ctlWorkerCreateCmd.Flags().StringVarP(&workerCreateName, "name", "n", "", "Worker name (required)")
	ctlWorkerCreateCmd.MarkFlagRequired("name")
	ctlWorkerCreateCmd.Flags().StringVar(&workerCreateDescription, "description", "", "Worker description")
	ctlWorkerCreateCmd.Flags().StringVar(&workerCreateMemory, "memory", "", "Worker memory content")
	ctlWorkerCreateCmd.Flags().StringVar(&workerCreateWorkDir, "work-dir", "", "Working directory path")

	ctlWorkerUpdateCmd.Flags().StringVar(&workerUpdateName, "name", "", "New name")
	ctlWorkerUpdateCmd.Flags().StringVar(&workerUpdateDescription, "description", "", "New description")
	ctlWorkerUpdateCmd.Flags().StringVar(&workerUpdateMemory, "memory", "", "New memory content")

	ctlWorkerDeleteCmd.Flags().BoolVar(&workerDeleteWorkDir, "delete-work-dir", false, "Also delete the worker's working directory from disk")

	ctlWorkerListCmd.Flags().StringVar(&workerListDepartment, "department", "", "Filter by department ID or name")
	ctlWorkerListCmd.Flags().BoolVar(&workerListNoRecursive, "no-recursive", false, "Only return workers directly in the department (not in child departments)")
	ctlWorkerCreateCmd.Flags().StringVar(&workerCreateDepartment, "department", "", "Department ID or name (comma-separated for multiple)")
	ctlWorkerUpdateCmd.Flags().StringVar(&workerUpdateDepartment, "department", "", "Department ID or name (comma-separated); replaces all associations. Pass empty string to clear.")
	ctlWorkerCreateCmd.Flags().StringVar(&workerCreateScopes, "scopes", "", "Permission scopes (comma-separated, e.g. read:workers,read:tasks)")
	ctlWorkerUpdateCmd.Flags().StringVar(&workerUpdateScopes, "scopes", "", "Permission scopes (comma-separated); replaces all scopes. Pass empty string to clear.")

	ctlWorkerCmd.AddCommand(
		ctlWorkerListCmd,
		ctlWorkerGetCmd,
		ctlWorkerStatusCmd,
		ctlWorkerCreateCmd,
		ctlWorkerUpdateCmd,
		ctlWorkerDeleteCmd,
	)
	ctlCmd.AddCommand(ctlWorkerCmd)
}
