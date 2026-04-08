package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

// toolSchema represents a single MCP tool definition returned by tools/list.
type toolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ToolSchemas returns the JSON Schema definitions for all Bee MCP tools.
// Exported so tests can verify the count and structure.
func ToolSchemas() []toolSchema {
	return beeToolSchemas()
}

// WorkerToolSchemas returns the JSON Schema definitions for Worker MCP tools.
func WorkerToolSchemas() []toolSchema { return workerToolSchemas() }

func beeToolSchemas() []toolSchema {
	return []toolSchema{
		{
			Name:        utils.ListWorkers,
			Description: "List all workers, optionally filtered by department",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"department_id": map[string]string{"type": "string", "description": "Filter by department ID or name"},
					"recursive":     map[string]any{"type": "boolean", "description": "Include workers in child departments (default: true)", "default": true},
				},
			},
		},
		{
			Name:        utils.GetWorker,
			Description: "Get a single worker by ID",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"worker_id"},
				"properties": map[string]any{
					"worker_id": map[string]string{"type": "string", "description": "Worker ID"},
				},
			},
		},
		{
			Name:        utils.CreateWorker,
			Description: "Create a new worker",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]any{
					"name":           map[string]string{"type": "string", "description": "Worker name"},
					"description":    map[string]string{"type": "string", "description": "Worker description"},
					"memory":         map[string]string{"type": "string", "description": "Worker memory content"},
					"work_dir":       map[string]string{"type": "string", "description": "Working directory path (optional, auto-assigned if empty)"},
					"department_ids": map[string]string{"type": "string", "description": "Comma-separated department IDs or names to associate the worker with"},
				},
			},
		},
		{
			Name:        utils.UpdateWorker,
			Description: "Update a worker's name, description, or memory (patch semantics: omitted fields unchanged)",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"worker_id"},
				"properties": map[string]any{
					"worker_id":      map[string]string{"type": "string", "description": "Worker ID"},
					"name":           map[string]string{"type": "string", "description": "New name"},
					"description":    map[string]string{"type": "string", "description": "New description"},
					"memory":         map[string]string{"type": "string", "description": "New memory content"},
					"department_ids": map[string]string{"type": "string", "description": "Comma-separated department IDs or names; replaces all existing associations. Empty string clears all."},
				},
			},
		},
		{
			Name:        utils.DeleteWorker,
			Description: "Delete a worker",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"worker_id"},
				"properties": map[string]any{
					"worker_id":       map[string]string{"type": "string", "description": "Worker ID"},
					"delete_work_dir": map[string]any{"type": "boolean", "description": "Also delete the worker's working directory from disk", "default": false},
				},
			},
		},
		{
			Name:        utils.CreateTask,
			Description: "Create a task assigning a worker to handle a user instruction from a message",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"message_id", "worker_id", "instruction", "type"},
				"properties": map[string]any{
					"message_id":        map[string]string{"type": "string", "description": "ID of the originating platform message"},
					"worker_id":         map[string]string{"type": "string", "description": "Worker ID to assign"},
					"instruction":       map[string]string{"type": "string", "description": "Specific instruction for the worker"},
					"type":              map[string]any{"type": "string", "enum": []string{"immediate", "countdown", "scheduled"}, "description": "Task type"},
					"scheduled_at":      map[string]string{"type": "integer", "description": "Unix ms; required for countdown, must be >= now+5s"},
					"cron_expr":         map[string]string{"type": "string", "description": "5-field cron expression; required for scheduled"},
				},
			},
		},
		{
			Name:        utils.ListTasks,
			Description: "List tasks filtered by message_id, session_key, and/or worker_id. message_id and session_key are mutually exclusive; at least one of message_id, session_key, or worker_id is required.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message_id":  map[string]string{"type": "string", "description": "Filter by message ID (mutually exclusive with session_key)"},
					"session_key": map[string]string{"type": "string", "description": "Filter by session key (mutually exclusive with message_id)"},
					"worker_id":   map[string]string{"type": "string", "description": "Filter by worker ID across all sessions; can be combined with session_key"},
					"status":      map[string]string{"type": "string", "description": "Optional status filter, supports comma-separated values e.g. 'pending,running'"},
					"type":        map[string]string{"type": "string", "description": "Optional type filter, supports comma-separated values e.g. 'scheduled' or 'immediate,countdown'"},
				},
			},
		},
		{
			Name:        utils.CancelTask,
			Description: "Cancel a pending or scheduled task",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"task_id"},
				"properties": map[string]any{
					"task_id": map[string]string{"type": "string", "description": "Task ID to cancel"},
				},
			},
		},
		{
			Name:        utils.SendMessage,
			Description: "Send a message to the user on the originating platform. Use message_id from the task metadata to identify the reply target. Supports sending media files (images, documents, audio, video) by providing a local file path.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"message_id"},
				"properties": map[string]any{
					"message_id": map[string]string{"type": "string", "description": "ID of the originating platform message (resolves platform and reply context)"},
					"content":    map[string]string{"type": "string", "description": "Text content to send (required unless media_path is provided)"},
					"media_path": map[string]string{"type": "string", "description": "Local file path to upload and send as media (image, file, audio, or video)"},
				},
			},
		},
		{
			Name:        utils.ClearSession,
			Description: "Cancel all active tasks (terminating running worker processes), clear dispatcher queues, and reset all session contexts for the given session. Use this to fully reset a conversation session.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"session_key"},
				"properties": map[string]any{
					"session_key": map[string]string{"type": "string", "description": "The session key to clear"},
					"force":       map[string]any{"type": "boolean", "description": "Skip confirmation when multiple workers are linked; default false", "default": false},
				},
			},
		},
		{
			Name:        utils.GetWorkerStatus,
			Description: "View a worker's current status: whether busy, current task, and pending task count.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"worker_id"},
				"properties": map[string]any{
					"worker_id": map[string]string{"type": "string", "description": "Worker ID"},
				},
			},
		},
		{
			Name:        utils.GetSystemOverview,
			Description: "View system overview: worker status distribution, task statistics, and the 5 most recent executions.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        utils.ListBeeExecutions,
			Description: "View bee's own execution history for self-reflection and improvement.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit": map[string]string{"type": "integer", "description": "Number of records to return, default 10"},
				},
			},
		},
		{
			Name:        utils.SaveMemory,
			Description: "Save or update a memory entry. Use scope='global' for global knowledge, or pass a session_key to store per-user preferences.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"scope", "key", "value"},
				"properties": map[string]any{
					"scope": map[string]string{"type": "string", "description": "Memory scope: 'global' or a session_key"},
					"key":   map[string]string{"type": "string", "description": "Memory key identifier, e.g. 'user_language_preference'"},
					"value": map[string]string{"type": "string", "description": "Memory value content"},
				},
			},
		},
		{
			Name:        utils.GetMemory,
			Description: "Read memory. Pass a key to get a single entry; omit key to get all entries in the scope (max 50).",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"scope"},
				"properties": map[string]any{
					"scope": map[string]string{"type": "string", "description": "Memory scope: 'global' or a session_key"},
					"key":   map[string]string{"type": "string", "description": "Memory key (optional; omit to return all entries in scope)"},
				},
			},
		},
		{
			Name:        utils.DeleteMemory,
			Description: "Delete a memory entry. Deleting a non-existent key is a no-op.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"scope", "key"},
				"properties": map[string]any{
					"scope": map[string]string{"type": "string", "description": "Memory scope"},
					"key":   map[string]string{"type": "string", "description": "Memory key identifier"},
				},
			},
		},
		{
			Name:        utils.ListSessionContexts,
			Description: "List all agents (bee and workers) that have active session contexts for a given session key.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"session_key"},
				"properties": map[string]any{
					"session_key": map[string]string{"type": "string", "description": "The session key to query"},
				},
			},
		},
		{
			Name:        utils.ClearWorkerSession,
			Description: "Reset one worker's Claude session context within a session, without affecting other workers or bee. Does not cancel tasks.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"session_key", "worker_id"},
				"properties": map[string]any{
					"session_key": map[string]string{"type": "string", "description": "The session key"},
					"worker_id":   map[string]string{"type": "string", "description": "Worker ID whose session context to delete"},
				},
			},
		},
		{
			Name:        utils.ListDepartments,
			Description: "List all departments as a tree structure",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        utils.GetDepartment,
			Description: "Get a department by ID or name",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": map[string]string{"type": "string", "description": "Department ID or name"},
				},
			},
		},
		{
			Name:        utils.CreateDepartment,
			Description: "Create a new department",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]any{
					"name":       map[string]string{"type": "string", "description": "Department name"},
					"parent_id":  map[string]string{"type": "string", "description": "Parent department ID or name"},
					"sort_order": map[string]string{"type": "integer", "description": "Display sort order"},
				},
			},
		},
		{
			Name:        utils.UpdateDepartment,
			Description: "Update a department (patch semantics: omitted fields unchanged). Setting parent_id moves the department; cannot move to root level.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id":         map[string]string{"type": "string", "description": "Department ID or name"},
					"name":       map[string]string{"type": "string", "description": "New name"},
					"parent_id":  map[string]string{"type": "string", "description": "New parent department ID or name"},
					"sort_order": map[string]string{"type": "integer", "description": "New sort order"},
				},
			},
		},
		{
			Name:        utils.DeleteDepartment,
			Description: "Delete a department. Fails if it has child departments or associated workers.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": map[string]string{"type": "string", "description": "Department ID or name"},
				},
			},
		},
	}
}

// workerToolNames is the allowlist of tools exposed to workers.
var workerToolNames = map[string]bool{
	utils.SendMessage:      true,
	utils.SaveMemory:       true,
	utils.GetMemory:        true,
	utils.DeleteMemory:     true,
}

func workerToolSchemas() []toolSchema {
	all := beeToolSchemas()
	out := make([]toolSchema, 0, len(workerToolNames))
	for _, s := range all {
		if workerToolNames[s.Name] {
			out = append(out, s)
		}
	}
	return out
}

// CallTool is exported for testing. Production code uses callToolFn via handleToolCall.
func (s *MCPServer) CallTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	return s.callToolFn(ctx, name, args)
}

// workerDisplayName returns the worker's configured name, falling back to the raw ID.
// Results are cached for the server's lifetime to avoid a DB round-trip on every send_message call.
func (s *MCPServer) workerDisplayName(workerID string) string {
	if v, ok := s.workerNameCache.Load(workerID); ok {
		return v.(string)
	}
	name := workerID
	if w, err := s.workerStore.GetByID(workerID); err == nil {
		name = w.Name
	} else {
		log.Debug("workerDisplayName: store lookup failed, falling back to ID", zap.String("workerID", workerID), zap.Error(err))
	}
	s.workerNameCache.Store(workerID, name)
	return name
}

// beeCallTool dispatches to the named tool handler and returns the result.
func (s *MCPServer) beeCallTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	switch name {
	case utils.ListWorkers:
		return s.toolListWorkers(args)
	case utils.GetWorker:
		return s.toolGetWorker(args)
	case utils.CreateWorker:
		return s.toolCreateWorker(args)
	case utils.UpdateWorker:
		return s.toolUpdateWorker(args)
	case utils.DeleteWorker:
		return s.toolDeleteWorker(args)
	case utils.CreateTask:
		return s.toolCreateTask(args)
	case utils.ListTasks:
		return s.toolListTasks(args)
	case utils.CancelTask:
		return s.toolCancelTask(args)
	case utils.SendMessage:
		return s.toolSendMessage(ctx, args)
	case utils.ClearSession:
		return s.toolClearSession(ctx, args)
	case utils.GetWorkerStatus:
		return s.toolGetWorkerStatus(args)
	case utils.GetSystemOverview:
		return s.toolGetSystemOverview(args)
	case utils.ListBeeExecutions:
		return s.toolListBeeExecutions(args)
	case utils.SaveMemory:
		return s.toolSaveMemory(args)
	case utils.GetMemory:
		return s.toolGetMemory(args)
	case utils.DeleteMemory:
		return s.toolDeleteMemory(args)
	case utils.ListSessionContexts:
		return s.toolListSessionContexts(args)
	case utils.ClearWorkerSession:
		return s.toolClearWorkerSession(args)
	case utils.ListDepartments:
		return s.toolListDepartments(args)
	case utils.GetDepartment:
		return s.toolGetDepartment(args)
	case utils.CreateDepartment:
		return s.toolCreateDepartment(args)
	case utils.UpdateDepartment:
		return s.toolUpdateDepartment(args)
	case utils.DeleteDepartment:
		return s.toolDeleteDepartment(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// workerCallTool delegates to beeCallTool after checking the worker allowlist.
func (s *MCPServer) workerCallTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	if !workerToolNames[name] {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return s.beeCallTool(ctx, name, args)
}

func (s *MCPServer) toolListWorkers(args json.RawMessage) (any, error) {
	var params struct {
		DepartmentID string `json:"department_id"`
		Recursive    *bool  `json:"recursive"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}

	var workers []model.Worker
	var err error

	if params.DepartmentID != "" {
		deptID, resolveErr := s.resolveDepartmentID(params.DepartmentID)
		if resolveErr != nil {
			return nil, resolveErr
		}
		recursive := params.Recursive == nil || *params.Recursive
		if recursive {
			workers, err = s.listWorkersRecursive(deptID)
		} else {
			workers, err = s.workerStore.GetByDepartmentID(deptID)
		}
	} else {
		workers, err = s.workerStore.List()
	}

	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	if workers == nil {
		workers = []model.Worker{}
	}
	return workers, nil
}

func (s *MCPServer) toolGetWorker(args json.RawMessage) (any, error) {
	var params struct {
		WorkerID string `json:"worker_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}
	return s.workerStore.GetByID(params.WorkerID)
}

func (s *MCPServer) toolCreateWorker(args json.RawMessage) (any, error) {
	var params struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		Memory        string `json:"memory"`
		WorkDir       string `json:"work_dir"`
		DepartmentIDs string `json:"department_ids"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	w, err := s.manager.CreateWorker(params.Name, params.Description, params.Memory, params.WorkDir)
	if err != nil {
		return nil, err
	}
	if params.DepartmentIDs != "" {
		if err := s.applyWorkerDepartments(w.ID, params.DepartmentIDs); err != nil {
			return nil, err
		}
	}
	return w, nil
}

func (s *MCPServer) toolUpdateWorker(args json.RawMessage) (any, error) {
	var params struct {
		WorkerID      string  `json:"worker_id"`
		Name          *string `json:"name"`
		Description   *string `json:"description"`
		Memory        *string `json:"memory"`
		DepartmentIDs *string `json:"department_ids"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}
	w, err := s.workerStore.GetByID(params.WorkerID)
	if err != nil {
		return nil, fmt.Errorf("worker not found: %w", err)
	}
	if params.Name != nil {
		w.Name = *params.Name
	}
	if params.Description != nil {
		w.Description = *params.Description
	}
	if params.Memory != nil {
		w.Memory = *params.Memory
	}
	w, err = s.workerStore.Update(w)
	if err != nil {
		return nil, err
	}
	if params.DepartmentIDs != nil {
		if err := s.applyWorkerDepartments(w.ID, *params.DepartmentIDs); err != nil {
			return nil, err
		}
	}
	return w, nil
}

func (s *MCPServer) toolDeleteWorker(args json.RawMessage) (any, error) {
	var params struct {
		WorkerID      string `json:"worker_id"`
		DeleteWorkDir bool   `json:"delete_work_dir"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}
	if err := s.manager.DeleteWorker(params.WorkerID, params.DeleteWorkDir); err != nil {
		return nil, err
	}
	return map[string]string{"status": "deleted"}, nil
}

func (s *MCPServer) toolCreateTask(args json.RawMessage) (any, error) {
	var params struct {
		MessageID       string `json:"message_id"`
		WorkerID        string `json:"worker_id"`
		Instruction     string `json:"instruction"`
		Type            string `json:"type"`
		ScheduledAt     *int64 `json:"scheduled_at"`
		CronExpr        string `json:"cron_expr"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.MessageID == "" {
		return nil, fmt.Errorf("message_id is required")
	}
	if params.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}
	if params.Instruction == "" {
		return nil, fmt.Errorf("instruction is required")
	}
	switch params.Type {
	case "immediate", "countdown", "scheduled":
	default:
		return nil, fmt.Errorf("type must be immediate, countdown, or scheduled")
	}

	nowMS := time.Now().UnixMilli()

	switch params.Type {
	case "countdown":
		if params.ScheduledAt == nil {
			return nil, fmt.Errorf("scheduled_at is required for countdown tasks")
		}
		if *params.ScheduledAt < nowMS+5000 {
			return nil, fmt.Errorf("scheduled_at must be at least 5 seconds in the future")
		}
	case "scheduled":
		if params.CronExpr == "" {
			return nil, fmt.Errorf("cron_expr is required for scheduled tasks")
		}
	}

	var nextRunAt *int64
	if params.Type == "scheduled" {
		sched, err := cron.ParseStandard(params.CronExpr)
		if err != nil {
			task := model.Task{
				MessageID:   params.MessageID,
				WorkerID:    params.WorkerID,
				Instruction: params.Instruction,
				Type:        params.Type,
				Status:      model.TaskStatusCancelled,
				CronExpr:    params.CronExpr,
				CreatedAt:   nowMS,
				UpdatedAt:   nowMS,
			}
			id, createErr := s.taskStore.Create(context.Background(), task)
			if createErr != nil {
				return nil, fmt.Errorf("create cancelled task: %w", createErr)
			}
			return map[string]string{"task_id": id, "status": "cancelled", "reason": "invalid cron_expr: " + err.Error()}, nil
		}
		next := sched.Next(time.Now()).UnixMilli()
		nextRunAt = &next
	}

	task := model.Task{
		MessageID:       params.MessageID,
		WorkerID:        params.WorkerID,
		Instruction:     params.Instruction,
		Type:            params.Type,
		Status:          model.TaskStatusPending,
		ScheduledAt:     params.ScheduledAt,
		CronExpr:        params.CronExpr,
		NextRunAt:       nextRunAt,
		CreatedAt: nowMS,
		UpdatedAt:       nowMS,
	}

	id, err := s.taskStore.Create(context.Background(), task)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return map[string]string{"task_id": id, "status": "pending"}, nil
}

func (s *MCPServer) toolListTasks(args json.RawMessage) (any, error) {
	var params struct {
		MessageID  string `json:"message_id"`
		SessionKey string `json:"session_key"`
		WorkerID   string `json:"worker_id"`
		Status     string `json:"status"`
		Type       string `json:"type"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.MessageID != "" && params.SessionKey != "" {
		return nil, fmt.Errorf("message_id and session_key are mutually exclusive")
	}
	if params.MessageID == "" && params.SessionKey == "" && params.WorkerID == "" {
		return nil, fmt.Errorf("at least one of message_id, session_key, or worker_id is required")
	}
	tasks, err := s.taskStore.List(context.Background(), store.TaskFilter{
		MessageID:  params.MessageID,
		SessionKey: params.SessionKey,
		WorkerID:   params.WorkerID,
		Status:     params.Status,
		Type:       params.Type,
	})
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	if tasks == nil {
		tasks = []model.Task{}
	}
	return tasks, nil
}

func (s *MCPServer) toolCancelTask(args json.RawMessage) (any, error) {
	var params struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	ctx := context.Background()

	// Stop running execution if any
	task, err := s.taskStore.GetByID(ctx, params.TaskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if task.ExecutionID != "" && s.execStopper != nil {
		if err := s.execStopper.StopExecution(task.ExecutionID); err != nil {
			log.Error("stop execution", zap.String("op", "cancel_task"), zap.String("executionID", task.ExecutionID), zap.Error(err))
		}
	}

	if err := s.taskStore.CancelTask(ctx, params.TaskID); err != nil {
		return nil, fmt.Errorf("cancel task: %w", err)
	}
	return map[string]string{"task_id": params.TaskID, "status": "cancelled"}, nil
}

func (s *MCPServer) toolSendMessage(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		MessageID string `json:"message_id"`
		Content   string `json:"content"`
		MediaPath string `json:"media_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.MessageID == "" {
		return nil, fmt.Errorf("message_id is required")
	}
	if params.Content == "" && params.MediaPath == "" {
		return nil, fmt.Errorf("at least one of 'content' or 'media_path' must be provided")
	}

	sourceType := store.SourceTypeBee
	sourceID := ""
	if workerID, _ := ctx.Value(CtxWorkerIDKey).(string); workerID != "" {
		sourceType = store.SourceTypeWorker
		sourceID = workerID
		if params.Content != "" {
			params.Content = s.workerDisplayName(workerID) + "\n" + params.Content
		}
	}

	stored, err := s.messageStore.GetByID(ctx, params.MessageID)
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}

	sender, ok := s.senders[stored.Platform]
	if !ok {
		return nil, fmt.Errorf("no sender registered for platform %q", stored.Platform)
	}

	replyTo := platform.InboundMessage{
		Platform:   stored.Platform,
		SessionKey: stored.SessionKey,
		Raw:        stored.Raw,
	}

	base := platform.OutboundMessage{
		ReplyTo:      replyTo,
		SourceType:   sourceType,
		SourceID:     sourceID,
		InboundMsgID: params.MessageID,
	}

	if params.Content != "" {
		outbound := base
		outbound.Content = params.Content
		if err := sender.Send(ctx, outbound); err != nil {
			return nil, fmt.Errorf("send text message: %w", err)
		}
	}

	if params.MediaPath != "" {
		outbound := base
		outbound.MediaPath = params.MediaPath
		if err := sender.Send(ctx, outbound); err != nil {
			return nil, fmt.Errorf("send media message: %w", err)
		}
	}

	return map[string]string{"status": "sent"}, nil
}

func (s *MCPServer) toolClearSession(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionKey string `json:"session_key"`
		Force      bool   `json:"force"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.SessionKey == "" {
		return nil, fmt.Errorf("session_key is required")
	}

	// Two-step confirmation: if more than one worker has a session context and
	// force is not set, return a confirmation prompt without clearing anything.
	if !params.Force {
		agents, err := s.sessionStore.ListSessionContexts(ctx, params.SessionKey)
		if err != nil {
			return nil, fmt.Errorf("list session contexts: %w", err)
		}
		var workers []map[string]string
		for _, a := range agents {
			if a.AgentType == "worker" {
				workers = append(workers, map[string]string{
					"worker_id": a.AgentID,
					"name":      a.Name,
				})
			}
		}
		if len(workers) > 1 {
			return map[string]any{
				"requires_confirmation": true,
				"worker_count":          len(workers),
				"linked_workers":        workers,
				"message":               fmt.Sprintf(i18n.M.Runtime.MCP.ClearSessionConfirm, len(workers)),
			}, nil
		}
	}

	// Step 1: Collect running tasks with execution IDs (before cancelling)
	runningTasks, err := s.taskStore.ListBySessionKey(ctx, params.SessionKey, "running", "")
	if err != nil {
		return nil, fmt.Errorf("list running tasks: %w", err)
	}

	// Step 2: Stop running worker processes
	for _, t := range runningTasks {
		if t.ExecutionID != "" {
			if err := s.execStopper.StopExecution(t.ExecutionID); err != nil {
				log.Error("stop execution", zap.String("op", "clear_session"), zap.String("executionID", t.ExecutionID), zap.Error(err))
			}
		}
	}

	// Step 3: Cancel all pending/running tasks in DB
	cancelled, err := s.taskStore.CancelBySessionKey(ctx, params.SessionKey)
	if err != nil {
		return nil, fmt.Errorf("cancel tasks: %w", err)
	}

	// Step 4: Clear dispatcher queues + session contexts
	if s.sessionClearer != nil {
		s.sessionClearer.ClearSession(params.SessionKey)
	}

	return map[string]any{
		"cancelled_tasks": cancelled,
		"cleared":         true,
	}, nil
}

func (s *MCPServer) toolGetWorkerStatus(args json.RawMessage) (any, error) {
	var p struct {
		WorkerID string `json:"worker_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}

	worker, err := s.workerStore.GetByID(p.WorkerID)
	if err != nil {
		return nil, fmt.Errorf("worker not found: %w", err)
	}

	ctx := context.Background()
	result := map[string]any{
		"worker_id":         worker.ID,
		"name":              worker.Name,
		"status":            string(worker.Status),
		"current_execution": nil,
	}

	runningExec, err := s.executionStore.GetRunningByWorkerID(worker.ID)
	if err == nil && runningExec != nil {
		execInfo := map[string]any{
			"id":          runningExec.ID,
			"task_id":     nil,
			"instruction": runningExec.TriggerInput,
			"started_at":  runningExec.StartedAt,
		}
		task, terr := s.taskStore.GetTaskByExecutionID(ctx, runningExec.ID)
		if terr == nil && task != nil {
			execInfo["task_id"] = task.ID
		}
		result["current_execution"] = execInfo
	}

	pendingCount, err := s.taskStore.CountPendingByWorkerID(ctx, p.WorkerID)
	if err != nil {
		pendingCount = 0
	}
	result["pending_tasks_count"] = pendingCount

	return result, nil
}

func (s *MCPServer) toolGetSystemOverview(_ json.RawMessage) (any, error) {
	// Worker counts
	workerCounts, err := s.workerStore.CountByStatus()
	if err != nil {
		return nil, fmt.Errorf("failed to get worker counts: %w", err)
	}
	total := 0
	for _, c := range workerCounts {
		total += c
	}

	// Task counts
	ctx := context.Background()
	taskCounts, err := s.taskStore.CountAllByStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get task counts: %w", err)
	}

	scheduledActive, _ := s.taskStore.CountScheduledActive(ctx)

	recentExecs, _ := s.executionStore.ListRecent(5)

	recentList := make([]map[string]any, 0, len(recentExecs))
	for _, e := range recentExecs {
		recentList = append(recentList, map[string]any{
			"id":           e.ID,
			"worker_name":  e.WorkerName,
			"status":       string(e.Status),
			"started_at":   e.StartedAt,
			"completed_at": e.CompletedAt,
		})
	}

	return map[string]any{
		"workers": map[string]any{
			"total":   total,
			"idle":    workerCounts["idle"],
			"working": workerCounts["working"],
			"error":   workerCounts["error"],
		},
		"tasks": map[string]any{
			"pending":          taskCounts["pending"],
			"running":          taskCounts["running"],
			"completed":        taskCounts["completed"],
			"failed":           taskCounts["failed"],
			"cancelled":        taskCounts["cancelled"],
			"scheduled_active": scheduledActive,
		},
		"recent_executions": recentList,
	}, nil
}

func (s *MCPServer) toolListBeeExecutions(args json.RawMessage) (any, error) {
	var p struct {
		Limit int `json:"limit"`
	}
	if args != nil {
		json.Unmarshal(args, &p) //nolint
	}
	if p.Limit <= 0 {
		p.Limit = 10
	}

	execs, err := s.executionStore.ListBeeExecutions(p.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list bee executions: %w", err)
	}

	results := make([]map[string]any, 0, len(execs))
	for _, e := range execs {
		triggerInput := e.TriggerInput
		if len(triggerInput) > 200 {
			triggerInput = triggerInput[:200]
		}
		result := e.Result
		if len(result) > 200 {
			result = result[:200]
		}
		results = append(results, map[string]any{
			"id":            e.ID,
			"trigger_input": triggerInput,
			"status":        string(e.Status),
			"started_at":    e.StartedAt,
			"completed_at":  e.CompletedAt,
			"result":        result,
		})
	}

	return results, nil
}

func (s *MCPServer) toolSaveMemory(args json.RawMessage) (any, error) {
	var p struct {
		Scope string `json:"scope"`
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Scope == "" || p.Key == "" || p.Value == "" {
		return nil, fmt.Errorf("scope, key, and value are required")
	}
	if err := s.memoryStore.Save(p.Scope, p.Key, p.Value); err != nil {
		return nil, fmt.Errorf("failed to save memory: %w", err)
	}
	return map[string]string{"status": "saved"}, nil
}

func (s *MCPServer) toolGetMemory(args json.RawMessage) (any, error) {
	var p struct {
		Scope string `json:"scope"`
		Key   string `json:"key"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Scope == "" {
		return nil, fmt.Errorf("scope is required")
	}
	if p.Key != "" {
		mem, err := s.memoryStore.Get(p.Scope, p.Key)
		if err != nil {
			return nil, fmt.Errorf("failed to get memory: %w", err)
		}
		if mem == nil {
			return map[string]string{"status": "not_found"}, nil
		}
		return mem, nil
	}
	memories, err := s.memoryStore.ListByScope(p.Scope, 50)
	if err != nil {
		return nil, fmt.Errorf("failed to list memories: %w", err)
	}
	return memories, nil
}

func (s *MCPServer) toolDeleteMemory(args json.RawMessage) (any, error) {
	var p struct {
		Scope string `json:"scope"`
		Key   string `json:"key"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Scope == "" || p.Key == "" {
		return nil, fmt.Errorf("scope and key are required")
	}
	if err := s.memoryStore.Delete(p.Scope, p.Key); err != nil {
		return nil, fmt.Errorf("failed to delete memory: %w", err)
	}
	return map[string]string{"status": "deleted"}, nil
}

func (s *MCPServer) toolListSessionContexts(args json.RawMessage) (any, error) {
	var params struct {
		SessionKey string `json:"session_key"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.SessionKey == "" {
		return nil, fmt.Errorf("session_key is required")
	}
	agents, err := s.sessionStore.ListSessionContexts(context.Background(), params.SessionKey)
	if err != nil {
		return nil, fmt.Errorf("list session contexts: %w", err)
	}
	return agents, nil
}

func (s *MCPServer) toolClearWorkerSession(args json.RawMessage) (any, error) {
	var params struct {
		SessionKey string `json:"session_key"`
		WorkerID   string `json:"worker_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.SessionKey == "" {
		return nil, fmt.Errorf("session_key is required")
	}
	if params.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}
	if params.WorkerID == store.BeeAgentID {
		return nil, fmt.Errorf("cannot clear bee session context with this tool, use clear_session instead")
	}

	ctx := context.Background()

	// Resolve worker name regardless of whether a session row exists.
	workerName := "(deleted)"
	if w, err := s.workerStore.GetByID(params.WorkerID); err == nil {
		workerName = w.Name
	}

	if err := s.sessionStore.DeleteWorkerSessionContext(ctx, params.SessionKey, params.WorkerID); err != nil {
		return nil, fmt.Errorf("delete worker session context: %w", err)
	}

	return map[string]any{
		"cleared":     true,
		"worker_id":   params.WorkerID,
		"worker_name": workerName,
	}, nil
}

func (s *MCPServer) toolListDepartments(_ json.RawMessage) (any, error) {
	all, err := s.departmentStore.ListAll()
	if err != nil {
		return nil, fmt.Errorf("list departments: %w", err)
	}
	tree := s.departmentStore.BuildTree(all)
	if tree == nil {
		tree = []model.DepartmentTree{}
	}
	return tree, nil
}

func (s *MCPServer) toolGetDepartment(args json.RawMessage) (any, error) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	deptID, err := s.resolveDepartmentID(params.ID)
	if err != nil {
		return nil, err
	}
	return s.departmentStore.GetByID(deptID)
}

func (s *MCPServer) toolCreateDepartment(args json.RawMessage) (any, error) {
	var params struct {
		Name      string `json:"name"`
		ParentID  string `json:"parent_id"`
		SortOrder int    `json:"sort_order"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	d := model.Department{
		Name:      params.Name,
		SortOrder: params.SortOrder,
	}
	if params.ParentID != "" {
		parentID, err := s.resolveDepartmentID(params.ParentID)
		if err != nil {
			return nil, fmt.Errorf("parent: %w", err)
		}
		d.ParentID = &parentID
	}
	return s.departmentStore.Create(d)
}

func (s *MCPServer) toolUpdateDepartment(args json.RawMessage) (any, error) {
	var params struct {
		ID        string  `json:"id"`
		Name      *string `json:"name"`
		ParentID  *string `json:"parent_id"`
		SortOrder *int    `json:"sort_order"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	deptID, err := s.resolveDepartmentID(params.ID)
	if err != nil {
		return nil, err
	}
	d, err := s.departmentStore.GetByID(deptID)
	if err != nil {
		return nil, fmt.Errorf("get department: %w", err)
	}
	if params.Name != nil {
		d.Name = *params.Name
	}
	if params.SortOrder != nil {
		d.SortOrder = *params.SortOrder
	}
	if params.ParentID != nil {
		resolvedParentID, err := s.resolveDepartmentID(*params.ParentID)
		if err != nil {
			return nil, fmt.Errorf("parent: %w", err)
		}
		if err := s.departmentStore.CheckCircularReference(d.ID, resolvedParentID); err != nil {
			return nil, err
		}
		d.ParentID = &resolvedParentID
	}
	return s.departmentStore.Update(d)
}

func (s *MCPServer) toolDeleteDepartment(args json.RawMessage) (any, error) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	deptID, err := s.resolveDepartmentID(params.ID)
	if err != nil {
		return nil, err
	}
	if err := s.departmentStore.Delete(deptID); err != nil {
		return nil, err
	}
	return map[string]string{"status": "deleted"}, nil
}

func (s *MCPServer) resolveDepartmentID(idOrName string) (string, error) {
	if _, err := s.departmentStore.GetByID(idOrName); err == nil {
		return idOrName, nil
	}
	all, err := s.departmentStore.ListAll()
	if err != nil {
		return "", fmt.Errorf("list departments: %w", err)
	}
	var matches []model.Department
	for _, d := range all {
		if d.Name == idOrName {
			matches = append(matches, d)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("department %q not found", idOrName)
	case 1:
		return matches[0].ID, nil
	default:
		deptMap := make(map[string]model.Department, len(all))
		for _, d := range all {
			deptMap[d.ID] = d
		}
		paths := make([]string, len(matches))
		for i, m := range matches {
			paths[i] = departmentAncestorPath(deptMap, m)
		}
		return "", fmt.Errorf("department name %q is ambiguous, matches: %s; use an ID instead",
			idOrName, strings.Join(paths, ", "))
	}
}

// resolveDepartmentIDs resolves a slice of ID-or-name strings to department IDs with a single
// ListAll call to avoid per-element DB queries.
func (s *MCPServer) resolveDepartmentIDs(idOrNames []string) ([]string, error) {
	if len(idOrNames) == 0 {
		return nil, nil
	}
	all, err := s.departmentStore.ListAll()
	if err != nil {
		return nil, fmt.Errorf("list departments: %w", err)
	}
	deptByID := make(map[string]model.Department, len(all))
	deptByName := make(map[string][]model.Department)
	for _, d := range all {
		deptByID[d.ID] = d
		deptByName[d.Name] = append(deptByName[d.Name], d)
	}

	ids := make([]string, 0, len(idOrNames))
	for _, v := range idOrNames {
		if _, ok := deptByID[v]; ok {
			ids = append(ids, v)
			continue
		}
		matches := deptByName[v]
		switch len(matches) {
		case 0:
			return nil, fmt.Errorf("department %q not found", v)
		case 1:
			ids = append(ids, matches[0].ID)
		default:
			paths := make([]string, len(matches))
			for i, m := range matches {
				paths[i] = departmentAncestorPath(deptByID, m)
			}
			return nil, fmt.Errorf("department name %q is ambiguous, matches: %s; use an ID instead",
				v, strings.Join(paths, ", "))
		}
	}
	return ids, nil
}

// applyWorkerDepartments resolves a comma-separated list of department IDs or names and
// replaces all department associations for the worker. An empty string clears all associations.
func (s *MCPServer) applyWorkerDepartments(workerID, deptIDsParam string) error {
	var deptIDs []string
	if deptIDsParam != "" {
		var err error
		deptIDs, err = s.resolveDepartmentIDs(utils.SplitAndTrim(deptIDsParam))
		if err != nil {
			return fmt.Errorf("set departments: %w", err)
		}
	}
	if err := s.departmentStore.SetWorkerDepartments(workerID, deptIDs); err != nil {
		return fmt.Errorf("set worker departments: %w", err)
	}
	return nil
}

func departmentAncestorPath(deptMap map[string]model.Department, d model.Department) string {
	var parts []string
	cur := d
	for {
		parts = append([]string{cur.Name}, parts...)
		if cur.ParentID == nil {
			break
		}
		parent, ok := deptMap[*cur.ParentID]
		if !ok {
			break
		}
		cur = parent
	}
	return strings.Join(parts, " > ")
}

func collectDescendantIDs(all []model.Department, rootID string) []string {
	childrenMap := make(map[string][]string)
	for _, d := range all {
		if d.ParentID != nil {
			childrenMap[*d.ParentID] = append(childrenMap[*d.ParentID], d.ID)
		}
	}
	var ids []string
	var dfs func(id string)
	dfs = func(id string) {
		ids = append(ids, id)
		for _, childID := range childrenMap[id] {
			dfs(childID)
		}
	}
	dfs(rootID)
	return ids
}

func (s *MCPServer) listWorkersRecursive(deptID string) ([]model.Worker, error) {
	all, err := s.departmentStore.ListAll()
	if err != nil {
		return nil, fmt.Errorf("list departments: %w", err)
	}
	deptIDs := collectDescendantIDs(all, deptID)
	workerIDs, err := s.departmentStore.GetWorkerIDsForDepartments(deptIDs)
	if err != nil {
		return nil, fmt.Errorf("get department workers: %w", err)
	}
	return s.workerStore.GetByIDs(workerIDs)
}
