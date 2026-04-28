package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/theopenbee/openbee/internal/domain/group"
	"github.com/theopenbee/openbee/internal/infra/model"
)

func (s *MCPServer) requireGroupTools() error {
	if s.groupManager == nil || s.groupStore == nil {
		return fmt.Errorf("group tools are not configured")
	}
	return nil
}

func (s *MCPServer) toolCreateGroup(args json.RawMessage) (any, error) {
	if err := s.requireGroupTools(); err != nil {
		return nil, err
	}
	var params struct {
		Name             string `json:"name"`
		Description      string `json:"description"`
		Constraints      string `json:"constraints"`
		WorkDir          string `json:"work_dir"`
		Engine           string `json:"engine"`
		EngineArgs       string `json:"engine_args"`
		PermissionScopes string `json:"permission_scopes"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	return s.groupManager.CreateGroup(group.CreateGroupParams{
		Name:             params.Name,
		Description:      params.Description,
		Constraints:      params.Constraints,
		WorkDir:          params.WorkDir,
		Engine:           params.Engine,
		EngineArgs:       params.EngineArgs,
		PermissionScopes: params.PermissionScopes,
	})
}

func (s *MCPServer) toolListGroups(_ json.RawMessage) (any, error) {
	if err := s.requireGroupTools(); err != nil {
		return nil, err
	}
	groups, err := s.groupStore.List()
	if groups == nil {
		groups = []model.Group{}
	}
	return groups, err
}

func (s *MCPServer) toolGetGroup(args json.RawMessage) (any, error) {
	if err := s.requireGroupTools(); err != nil {
		return nil, err
	}
	var params struct {
		GroupID string `json:"group_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.GroupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}
	g, err := s.groupStore.GetByID(params.GroupID)
	if err != nil {
		return nil, fmt.Errorf("group not found: %w", err)
	}
	members, err := s.groupStore.ListMembers(g.ID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	if members == nil {
		members = []model.MemberBrief{}
	}
	return model.GroupWithMembers{Group: g, Members: members}, nil
}

func (s *MCPServer) toolUpdateGroup(args json.RawMessage) (any, error) {
	if err := s.requireGroupTools(); err != nil {
		return nil, err
	}
	var params struct {
		GroupID          string  `json:"group_id"`
		Name             *string `json:"name"`
		Description      *string `json:"description"`
		Constraints      *string `json:"constraints"`
		WorkDir          *string `json:"work_dir"`
		Engine           *string `json:"engine"`
		EngineArgs       *string `json:"engine_args"`
		PermissionScopes *string `json:"permission_scopes"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.GroupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}
	g, err := s.groupStore.GetByID(params.GroupID)
	if err != nil {
		return nil, fmt.Errorf("group not found: %w", err)
	}
	if params.Name != nil {
		g.Name = *params.Name
	}
	if params.Description != nil {
		g.Description = *params.Description
	}
	if params.Constraints != nil {
		g.Constraints = *params.Constraints
	}
	if params.WorkDir != nil {
		g.WorkDir = *params.WorkDir
	}
	if params.Engine != nil {
		g.Engine = *params.Engine
	}
	if params.EngineArgs != nil {
		g.EngineArgs = *params.EngineArgs
	}
	if params.PermissionScopes != nil {
		g.PermissionScopes = *params.PermissionScopes
	}
	return s.groupManager.UpdateGroup(g)
}

func (s *MCPServer) toolDeleteGroup(args json.RawMessage) (any, error) {
	if err := s.requireGroupTools(); err != nil {
		return nil, err
	}
	var params struct {
		GroupID       string `json:"group_id"`
		DeleteWorkDir bool   `json:"delete_work_dir"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.GroupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}
	if err := s.groupManager.DeleteGroup(params.GroupID, params.DeleteWorkDir); err != nil {
		return nil, err
	}
	return map[string]string{"status": "deleted"}, nil
}

func (s *MCPServer) toolAddGroupMember(args json.RawMessage) (any, error) {
	if err := s.requireGroupTools(); err != nil {
		return nil, err
	}
	var params struct {
		GroupID  string `json:"group_id"`
		WorkerID string `json:"worker_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.GroupID == "" || params.WorkerID == "" {
		return nil, fmt.Errorf("group_id and worker_id are required")
	}
	if err := s.groupManager.AddMember(params.GroupID, params.WorkerID); err != nil {
		return nil, err
	}
	return map[string]string{"status": "added"}, nil
}

func (s *MCPServer) toolRemoveGroupMember(args json.RawMessage) (any, error) {
	if err := s.requireGroupTools(); err != nil {
		return nil, err
	}
	var params struct {
		GroupID  string `json:"group_id"`
		WorkerID string `json:"worker_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.GroupID == "" || params.WorkerID == "" {
		return nil, fmt.Errorf("group_id and worker_id are required")
	}
	if err := s.groupManager.RemoveMember(params.GroupID, params.WorkerID); err != nil {
		return nil, err
	}
	return map[string]string{"status": "removed"}, nil
}

func (s *MCPServer) toolListGroupMembers(args json.RawMessage) (any, error) {
	if err := s.requireGroupTools(); err != nil {
		return nil, err
	}
	var params struct {
		GroupID string `json:"group_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.GroupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}
	members, err := s.groupStore.ListMembers(params.GroupID)
	if members == nil {
		members = []model.MemberBrief{}
	}
	return members, err
}

func (s *MCPServer) toolDispatchSubtask(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		ParentTaskID string `json:"parent_task_id"`
		WorkerID     string `json:"worker_id"`
		Instruction  string `json:"instruction"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.ParentTaskID == "" || params.WorkerID == "" {
		return nil, fmt.Errorf("parent_task_id and worker_id are required")
	}
	parent, err := s.taskStore.GetByID(ctx, params.ParentTaskID)
	if err != nil {
		return nil, fmt.Errorf("parent task not found: %w", err)
	}
	if parent.AgentKind != model.AgentKindGroup {
		return nil, fmt.Errorf("parent task is not a group task")
	}
	subID, err := s.taskStore.Create(ctx, model.Task{
		MessageID:    parent.MessageID,
		WorkerID:     params.WorkerID,
		Instruction:  params.Instruction,
		Type:         model.TaskTypeImmediate,
		Status:       model.TaskStatusPending,
		ParentTaskID: parent.ID,
		RootTaskID:   parent.RootTaskID,
		AgentKind:    model.AgentKindWorker,
	})
	if err != nil {
		return nil, err
	}
	return map[string]string{"subtask_id": subID}, nil
}

func (s *MCPServer) toolListSubtasks(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	tasks, err := s.taskStore.ListByRoot(ctx, params.TaskID)
	if tasks == nil {
		tasks = []model.Task{}
	}
	return tasks, err
}

func (s *MCPServer) toolSuspendTask(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if err := s.taskStore.MarkWaitingSubtasks(ctx, params.TaskID); err != nil {
		return nil, err
	}
	if s.subtaskAllNotifier != nil {
		if allTerminal, err := s.allSubtasksTerminal(ctx, params.TaskID); err == nil && allTerminal {
			s.subtaskAllNotifier.NotifyAllSubtasksTerminal(ctx, params.TaskID)
		}
	}
	return map[string]string{"status": model.TaskStatusWaitingSubtasks}, nil
}

func (s *MCPServer) toolMarkTaskSuccess(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if active, err := s.hasActiveSubtasks(ctx, params.TaskID); err != nil {
		return nil, err
	} else if active {
		return nil, fmt.Errorf("cannot mark success while subtasks are active")
	}
	if err := s.taskStore.CompleteTask(ctx, params.TaskID); err != nil {
		return nil, err
	}
	return map[string]string{"status": model.TaskStatusCompleted}, nil
}

func (s *MCPServer) toolMarkTaskFailed(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		TaskID string `json:"task_id"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if err := s.taskStore.FailTask(ctx, params.TaskID); err != nil {
		return nil, err
	}
	if err := s.cancelActiveSubtasks(ctx, params.TaskID); err != nil {
		return nil, err
	}
	return map[string]string{"status": model.TaskStatusFailed}, nil
}

func (s *MCPServer) allSubtasksTerminal(ctx context.Context, rootID string) (bool, error) {
	tasks, err := s.taskStore.ListByRoot(ctx, rootID)
	if err != nil {
		return false, err
	}
	if len(tasks) <= 1 {
		return false, nil
	}
	for _, t := range tasks {
		if t.ID == rootID {
			continue
		}
		if isActiveTaskStatus(t.Status) {
			return false, nil
		}
	}
	return true, nil
}

func (s *MCPServer) hasActiveSubtasks(ctx context.Context, rootID string) (bool, error) {
	tasks, err := s.taskStore.ListByRoot(ctx, rootID)
	if err != nil {
		return false, err
	}
	for _, t := range tasks {
		if t.ID != rootID && isActiveTaskStatus(t.Status) {
			return true, nil
		}
	}
	return false, nil
}

func (s *MCPServer) cancelActiveSubtasks(ctx context.Context, rootID string) error {
	tasks, err := s.taskStore.ListByRoot(ctx, rootID)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.ID == rootID || !isActiveTaskStatus(t.Status) {
			continue
		}
		if err := s.taskStore.CancelTask(ctx, t.ID); err != nil {
			return err
		}
	}
	return nil
}

func isActiveTaskStatus(status string) bool {
	return status == model.TaskStatusPending ||
		status == model.TaskStatusRunning ||
		status == model.TaskStatusWaitingSubtasks
}
