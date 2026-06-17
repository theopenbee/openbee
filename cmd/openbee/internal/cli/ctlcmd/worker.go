package ctlcmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

const engineArgsFlagName = "engine-args"

func newWorkerCommand(run Runner) *cobra.Command {
	subs := i18n.M.Cmd.CtlWorker
	cmd := &cobra.Command{
		Use:   "worker",
		Short: subs.Short,
	}

	cmd.AddCommand(
		newWorkerListCommand(run, subs.Sub("list")),
		newWorkerGetCommand(run, subs.Sub("get")),
		newWorkerStatusCommand(run, subs.Sub("status")),
		newWorkerCreateCommand(run, subs.Sub("create")),
		newWorkerUpdateCommand(run, subs.Sub("update")),
		newWorkerDeleteCommand(run, subs.Sub("delete")),
	)
	return cmd
}

func newWorkerListCommand(run Runner, short string) *cobra.Command {
	var (
		department  string
		noRecursive bool
		name        string
		id          string
		page        int
		pageSize    int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: short,
		RunE: func(c *cobra.Command, args []string) error {
			a := map[string]any{}
			if department != "" {
				a["department_id"] = department
				if noRecursive {
					a["recursive"] = false
				}
			}
			setIfNonEmpty(a, "name", name)
			setIfNonEmpty(a, "id", id)
			setIfPositive(a, "page", page)
			setIfPositive(a, "page_size", pageSize)
			return run(utils.ListWorkers, a)
		},
	}
	cmd.Flags().StringVar(&department, "department", "", "Filter by department ID or name")
	cmd.Flags().BoolVar(&noRecursive, "no-recursive", false, "Only return workers directly in the department (not in child departments)")
	cmd.Flags().StringVar(&name, "name", "", "Filter by name (case-insensitive partial match)")
	cmd.Flags().StringVar(&id, "id", "", "Filter by exact worker ID")
	cmd.Flags().IntVar(&page, "page", 0, "Page number (default: 1)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "Page size (default: 50, max: 200)")
	return cmd
}

func newWorkerGetCommand(run Runner, short string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return run(utils.GetWorker, map[string]any{"worker_id": args[0]})
		},
	}
}

func newWorkerStatusCommand(run Runner, short string) *cobra.Command {
	return &cobra.Command{
		Use:   "status <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return run(utils.GetWorkerStatus, map[string]any{"worker_id": args[0]})
		},
	}
}

func newWorkerCreateCommand(run Runner, short string) *cobra.Command {
	var (
		name        string
		description string
		constraints string
		workDir     string
		engine      string
		department  string
		scopes      string
		engineArgs  []string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: short,
		RunE: func(c *cobra.Command, args []string) error {
			a := map[string]any{"name": name}
			setIfNonEmpty(a, "description", description)
			setIfNonEmpty(a, "constraints", constraints)
			setIfNonEmpty(a, "work_dir", workDir)
			setIfNonEmpty(a, "engine", engine)
			setIfNonEmpty(a, "department_ids", department)
			setIfNonEmpty(a, "permission_scopes", scopes)
			if len(engineArgs) > 0 {
				parsed, err := parseEngineArgsFlag(engineArgs)
				if err != nil {
					return err
				}
				a["engine_args"] = parsed
			}
			return run(utils.CreateWorker, a)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "", "Worker name (required)")
	cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&description, "description", "", "Worker description")
	cmd.Flags().StringVar(&constraints, "constraints", "", "Worker constraints content")
	cmd.Flags().StringVar(&workDir, "work-dir", "", "Working directory path")
	cmd.Flags().StringVar(&engine, "engine", "", "AI engine to use (e.g. claude, codex, pi); leave empty for server default")
	cmd.Flags().StringVar(&department, "department", "", "Department ID or name (comma-separated for multiple)")
	cmd.Flags().StringVar(&scopes, "scopes", "", "Permission scopes (comma-separated, e.g. read:workers,read:tasks)")
	cmd.Flags().StringArrayVar(&engineArgs, engineArgsFlagName, nil, "Extra CLI args per engine, e.g. \"claude=--model claude-sonnet-4-5 --effort high\" (repeatable)")
	return cmd
}

func newWorkerUpdateCommand(run Runner, short string) *cobra.Command {
	var (
		name        string
		description string
		constraints string
		workDir     string
		engine      string
		department  string
		scopes      string
		engineArgs  []string
	)
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			a := map[string]any{"worker_id": args[0]}
			setIfFlagChanged(c, a, "name", "name", name)
			setIfFlagChanged(c, a, "description", "description", description)
			setIfFlagChanged(c, a, "constraints", "constraints", constraints)
			setIfFlagChanged(c, a, "work-dir", "work_dir", workDir)
			setIfFlagChanged(c, a, "engine", "engine", engine)
			setIfFlagChanged(c, a, "department", "department_ids", department)
			setIfFlagChanged(c, a, "scopes", "permission_scopes", scopes)
			if c.Flags().Changed(engineArgsFlagName) {
				parsed, err := parseEngineArgsFlag(engineArgs)
				if err != nil {
					return err
				}
				a["engine_args"] = parsed
			}
			return run(utils.UpdateWorker, a)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "New name")
	cmd.Flags().StringVar(&description, "description", "", "New description")
	cmd.Flags().StringVar(&constraints, "constraints", "", "New constraints content")
	cmd.Flags().StringVar(&workDir, "work-dir", "", "New working directory path")
	cmd.Flags().StringVar(&engine, "engine", "", "AI engine to use (e.g. claude, codex, pi); leave empty to keep unchanged")
	cmd.Flags().StringVar(&department, "department", "", "Department ID or name (comma-separated); replaces all associations. Pass empty string to clear.")
	cmd.Flags().StringVar(&scopes, "scopes", "", "Permission scopes (comma-separated); replaces all scopes. Pass empty string to clear.")
	cmd.Flags().StringArrayVar(&engineArgs, engineArgsFlagName, nil, "Extra CLI args per engine, e.g. \"claude=--model claude-opus-4-7\" (repeatable); pass \"claude=\" to clear")
	return cmd
}

func newWorkerDeleteCommand(run Runner, short string) *cobra.Command {
	var deleteWorkDir bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return run(utils.DeleteWorker, map[string]any{
				"worker_id":       args[0],
				"delete_work_dir": deleteWorkDir,
			})
		},
	}
	cmd.Flags().BoolVar(&deleteWorkDir, "delete-work-dir", false, "Also delete the worker's working directory from disk")
	return cmd
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
