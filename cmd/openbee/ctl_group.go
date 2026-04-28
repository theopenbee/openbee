package main

import (
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

var ctlGroupCmd = &cobra.Command{Use: "group", Short: ""}

var (
	groupCreateName        string
	groupCreateDescription string
	groupCreateConstraints string
	groupCreateEngine      string
	groupCreateScopes      string
)

var ctlGroupCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new group",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"name": groupCreateName}
		if groupCreateDescription != "" {
			a["description"] = groupCreateDescription
		}
		if groupCreateConstraints != "" {
			a["constraints"] = groupCreateConstraints
		}
		if groupCreateEngine != "" {
			a["engine"] = groupCreateEngine
		}
		if groupCreateScopes != "" {
			a["permission_scopes"] = groupCreateScopes
		}
		return ctlRun(utils.CreateGroup, a)
	},
}

var ctlGroupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List groups",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.ListGroups, map[string]any{})
	},
}

var ctlGroupGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a group by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.GetGroup, map[string]any{"group_id": args[0]})
	},
}

var ctlGroupDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a group",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"group_id": args[0]}
		if groupDeleteWorkDir {
			a["delete_work_dir"] = true
		}
		return ctlRun(utils.DeleteGroup, a)
	},
}

var groupDeleteWorkDir bool

var ctlGroupMemberCmd = &cobra.Command{Use: "member", Short: ""}

var (
	groupMemberGroupID  string
	groupMemberWorkerID string
)

var ctlGroupMemberAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a worker to a group",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.AddGroupMember, map[string]any{
			"group_id":  groupMemberGroupID,
			"worker_id": groupMemberWorkerID,
		})
	},
}

var ctlGroupMemberRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a worker from a group",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.RemoveGroupMember, map[string]any{
			"group_id":  groupMemberGroupID,
			"worker_id": groupMemberWorkerID,
		})
	},
}

var ctlGroupMemberListCmd = &cobra.Command{
	Use:   "list",
	Short: "List members of a group",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ctlRun(utils.ListGroupMembers, map[string]any{"group_id": groupMemberGroupID})
	},
}

func init() {
	ctlGroupCreateCmd.Flags().StringVar(&groupCreateName, "name", "", "Group name (required)")
	ctlGroupCreateCmd.Flags().StringVar(&groupCreateDescription, "description", "", "Group description")
	ctlGroupCreateCmd.Flags().StringVar(&groupCreateConstraints, "constraints", "", "Work constraints")
	ctlGroupCreateCmd.Flags().StringVar(&groupCreateEngine, "engine", "", "Engine name override")
	ctlGroupCreateCmd.Flags().StringVar(&groupCreateScopes, "permission-scopes", "", "Permission scopes")
	ctlGroupCreateCmd.MarkFlagRequired("name") //nolint:errcheck

	ctlGroupDeleteCmd.Flags().BoolVar(&groupDeleteWorkDir, "delete-work-dir", false, "Also delete the group's work_dir on disk")

	for _, c := range []*cobra.Command{ctlGroupMemberAddCmd, ctlGroupMemberRemoveCmd, ctlGroupMemberListCmd} {
		c.Flags().StringVar(&groupMemberGroupID, "group", "", "Group ID (required)")
		c.MarkFlagRequired("group") //nolint:errcheck
	}
	ctlGroupMemberAddCmd.Flags().StringVar(&groupMemberWorkerID, "worker", "", "Worker ID (required)")
	ctlGroupMemberAddCmd.MarkFlagRequired("worker")    //nolint:errcheck
	ctlGroupMemberRemoveCmd.Flags().StringVar(&groupMemberWorkerID, "worker", "", "Worker ID (required)")
	ctlGroupMemberRemoveCmd.MarkFlagRequired("worker") //nolint:errcheck

	ctlGroupMemberCmd.AddCommand(ctlGroupMemberAddCmd, ctlGroupMemberRemoveCmd, ctlGroupMemberListCmd)
	ctlGroupCmd.AddCommand(ctlGroupCreateCmd, ctlGroupListCmd, ctlGroupGetCmd, ctlGroupDeleteCmd, ctlGroupMemberCmd)
	ctlCmd.AddCommand(ctlGroupCmd)
}
