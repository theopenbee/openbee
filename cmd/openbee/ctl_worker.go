package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

const engineArgsFlagName = "engine-args"

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
		if workerListName != "" {
			a["name"] = workerListName
		}
		if workerListID != "" {
			a["id"] = workerListID
		}
		if workerListPage > 0 {
			a["page"] = workerListPage
		}
		if workerListPageSize > 0 {
			a["page_size"] = workerListPageSize
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
	workerCreateConstraints string
	workerCreateWorkDir     string
	workerCreateEngine      string
	workerCreateScopes      string
	workerCreateEngineArgs  []string
)

var ctlWorkerCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new worker",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"name": workerCreateName}
		if workerCreateDescription != "" {
			a["description"] = workerCreateDescription
		}
		if workerCreateConstraints != "" {
			a["constraints"] = workerCreateConstraints
		}
		if workerCreateWorkDir != "" {
			a["work_dir"] = workerCreateWorkDir
		}
		if workerCreateEngine != "" {
			a["engine"] = workerCreateEngine
		}
		if workerCreateDepartment != "" {
			a["department_ids"] = workerCreateDepartment
		}
		if workerCreateScopes != "" {
			a["permission_scopes"] = workerCreateScopes
		}
		if len(workerCreateEngineArgs) > 0 {
			parsed, err := parseEngineArgsFlag(workerCreateEngineArgs)
			if err != nil {
				return err
			}
			a["engine_args"] = parsed
		}
		return ctlRun(utils.CreateWorker, a)
	},
}

var (
	workerUpdateName        string
	workerUpdateDescription string
	workerUpdateConstraints string
	workerUpdateEngine      string
	workerUpdateScopes      string
	workerUpdateEngineArgs  []string
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
		if cmd.Flags().Changed("constraints") {
			a["constraints"] = workerUpdateConstraints
		}
		if cmd.Flags().Changed("engine") {
			a["engine"] = workerUpdateEngine
		}
		if cmd.Flags().Changed("department") {
			a["department_ids"] = workerUpdateDepartment
		}
		if cmd.Flags().Changed("scopes") {
			a["permission_scopes"] = workerUpdateScopes
		}
		if cmd.Flags().Changed(engineArgsFlagName) {
			parsed, err := parseEngineArgsFlag(workerUpdateEngineArgs)
			if err != nil {
				return err
			}
			a["engine_args"] = parsed
		}
		return ctlRun(utils.UpdateWorker, a)
	},
}

var (
	workerListDepartment   string
	workerListNoRecursive  bool
	workerListName         string
	workerListID           string
	workerListPage         int
	workerListPageSize     int
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
	ctlWorkerCreateCmd.Flags().StringVar(&workerCreateConstraints, "constraints", "", "Worker constraints content")
	ctlWorkerCreateCmd.Flags().StringVar(&workerCreateWorkDir, "work-dir", "", "Working directory path")
	ctlWorkerCreateCmd.Flags().StringVar(&workerCreateEngine, "engine", "", "AI engine to use (e.g. claude, codex, pi, kimi); leave empty for server default")

	ctlWorkerUpdateCmd.Flags().StringVar(&workerUpdateName, "name", "", "New name")
	ctlWorkerUpdateCmd.Flags().StringVar(&workerUpdateDescription, "description", "", "New description")
	ctlWorkerUpdateCmd.Flags().StringVar(&workerUpdateConstraints, "constraints", "", "New constraints content")
	ctlWorkerUpdateCmd.Flags().StringVar(&workerUpdateEngine, "engine", "", "AI engine to use (e.g. claude, codex, pi, kimi); leave empty to keep unchanged")

	ctlWorkerDeleteCmd.Flags().BoolVar(&workerDeleteWorkDir, "delete-work-dir", false, "Also delete the worker's working directory from disk")

	ctlWorkerListCmd.Flags().StringVar(&workerListDepartment, "department", "", "Filter by department ID or name")
	ctlWorkerListCmd.Flags().BoolVar(&workerListNoRecursive, "no-recursive", false, "Only return workers directly in the department (not in child departments)")
	ctlWorkerListCmd.Flags().StringVar(&workerListName, "name", "", "Filter by name (case-insensitive partial match)")
	ctlWorkerListCmd.Flags().StringVar(&workerListID, "id", "", "Filter by exact worker ID")
	ctlWorkerListCmd.Flags().IntVar(&workerListPage, "page", 0, "Page number (default: 1)")
	ctlWorkerListCmd.Flags().IntVar(&workerListPageSize, "page-size", 0, "Page size (default: 50, max: 200)")
	ctlWorkerCreateCmd.Flags().StringVar(&workerCreateDepartment, "department", "", "Department ID or name (comma-separated for multiple)")
	ctlWorkerUpdateCmd.Flags().StringVar(&workerUpdateDepartment, "department", "", "Department ID or name (comma-separated); replaces all associations. Pass empty string to clear.")
	ctlWorkerCreateCmd.Flags().StringVar(&workerCreateScopes, "scopes", "", "Permission scopes (comma-separated, e.g. read:workers,read:tasks)")
	ctlWorkerUpdateCmd.Flags().StringVar(&workerUpdateScopes, "scopes", "", "Permission scopes (comma-separated); replaces all scopes. Pass empty string to clear.")
	ctlWorkerCreateCmd.Flags().StringArrayVar(&workerCreateEngineArgs, engineArgsFlagName, nil, "Extra CLI args per engine, e.g. \"claude=--model claude-sonnet-4-5 --effort high\" (repeatable)")
	ctlWorkerUpdateCmd.Flags().StringArrayVar(&workerUpdateEngineArgs, engineArgsFlagName, nil, "Extra CLI args per engine, e.g. \"claude=--model claude-opus-4-7\" (repeatable); pass \"claude=\" to clear")

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

func parseEngineArgsFlag(entries []string) (map[string]string, error) {
	result := make(map[string]string, len(entries))
	for _, entry := range entries {
		engine, args, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("invalid --engine-args entry %q: expected format \"engine=flags\"", entry)
		}
		result[engine] = args
	}
	return result, nil
}
