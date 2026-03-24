package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/store"
	"github.com/theopenbee/openbee/internal/toolnames"
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
			Name:        toolnames.ListWorkers,
			Description: "List all workers",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        toolnames.GetWorker,
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
			Name:        toolnames.CreateWorker,
			Description: "Create a new worker",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]any{
					"name":        map[string]string{"type": "string", "description": "Worker name"},
					"description": map[string]string{"type": "string", "description": "Worker description"},
					"memory":      map[string]string{"type": "string", "description": "Worker memory content"},
					"work_dir":    map[string]string{"type": "string", "description": "Working directory path (optional, auto-assigned if empty)"},
				},
			},
		},
		{
			Name:        toolnames.UpdateWorker,
			Description: "Update a worker's name, description, or memory (patch semantics: omitted fields unchanged)",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"worker_id"},
				"properties": map[string]any{
					"worker_id":   map[string]string{"type": "string", "description": "Worker ID"},
					"name":        map[string]string{"type": "string", "description": "New name"},
					"description": map[string]string{"type": "string", "description": "New description"},
					"memory":      map[string]string{"type": "string", "description": "New memory content"},
				},
			},
		},
		{
			Name:        toolnames.DeleteWorker,
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
			Name:        toolnames.CreateTask,
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
			Name:        toolnames.ListTasks,
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
			Name:        toolnames.CancelTask,
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
			Name:        toolnames.MarkTaskComplete,
			Description: "Mark a task as successfully completed",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"task_id"},
				"properties": map[string]any{
					"task_id": map[string]string{"type": "string", "description": "Task ID to mark as completed"},
				},
			},
		},
		{
			Name:        toolnames.SendMessage,
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
			Name:        toolnames.ClearSession,
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
			Name:        toolnames.GetWorkerStatus,
			Description: "查看员工的当前状态，包括是否在工作、正在执行什么任务、待处理任务数量。",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"worker_id"},
				"properties": map[string]any{
					"worker_id": map[string]string{"type": "string", "description": "员工ID"},
				},
			},
		},
		{
			Name:        toolnames.GetSystemOverview,
			Description: "查看系统整体概况：员工状态分布、任务状态统计、最近5条执行记录。",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        toolnames.ListBeeExecutions,
			Description: "查看 bee 自己的执行历史记录，用于自我反思和改进。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit": map[string]string{"type": "integer", "description": "返回记录数量，默认10"},
				},
			},
		},
		{
			Name:        toolnames.SaveMemory,
			Description: "保存或更新一条记忆。scope 为 'global' 表示全局经验，或传入 session_key 表示特定用户的偏好。",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"scope", "key", "value"},
				"properties": map[string]any{
					"scope": map[string]string{"type": "string", "description": "记忆范围：'global' 或 session_key"},
					"key":   map[string]string{"type": "string", "description": "记忆标识符，如 'user_language_preference'"},
					"value": map[string]string{"type": "string", "description": "记忆内容"},
				},
			},
		},
		{
			Name:        toolnames.GetMemory,
			Description: "读取记忆。传入 key 返回单条记忆，不传 key 返回该 scope 下所有记忆（最多50条）。",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"scope"},
				"properties": map[string]any{
					"scope": map[string]string{"type": "string", "description": "记忆范围：'global' 或 session_key"},
					"key":   map[string]string{"type": "string", "description": "记忆标识符（可选，不传则返回该范围下所有记忆）"},
				},
			},
		},
		{
			Name:        toolnames.DeleteMemory,
			Description: "删除一条记忆。删除不存在的记忆不会报错。",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"scope", "key"},
				"properties": map[string]any{
					"scope": map[string]string{"type": "string", "description": "记忆范围"},
					"key":   map[string]string{"type": "string", "description": "记忆标识符"},
				},
			},
		},
		{
			Name:        toolnames.ListSessionContexts,
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
			Name:        toolnames.ClearWorkerSession,
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
	}
}

func workerToolSchemas() []toolSchema {
	return []toolSchema{
		{
			Name:        toolnames.MarkTaskComplete,
			Description: "Mark a task as successfully completed",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"task_id"},
				"properties": map[string]any{
					"task_id": map[string]string{"type": "string", "description": "Task ID to mark as completed"},
				},
			},
		},
		{
			Name:        toolnames.SendMessage,
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
			Name:        toolnames.SaveMemory,
			Description: "保存或更新一条记忆。scope 为 'global' 表示全局经验，或传入 session_key 表示特定用户的偏好。",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"scope", "key", "value"},
				"properties": map[string]any{
					"scope": map[string]string{"type": "string", "description": "记忆范围：'global' 或 session_key"},
					"key":   map[string]string{"type": "string", "description": "记忆标识符，如 'user_language_preference'"},
					"value": map[string]string{"type": "string", "description": "记忆内容"},
				},
			},
		},
		{
			Name:        toolnames.GetMemory,
			Description: "读取记忆。传入 key 返回单条记忆，不传 key 返回该 scope 下所有记忆（最多50条）。",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"scope"},
				"properties": map[string]any{
					"scope": map[string]string{"type": "string", "description": "记忆范围：'global' 或 session_key"},
					"key":   map[string]string{"type": "string", "description": "记忆标识符（可选，不传则返回该范围下所有记忆）"},
				},
			},
		},
		{
			Name:        toolnames.DeleteMemory,
			Description: "删除一条记忆。删除不存在的记忆不会报错。",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"scope", "key"},
				"properties": map[string]any{
					"scope": map[string]string{"type": "string", "description": "记忆范围"},
					"key":   map[string]string{"type": "string", "description": "记忆标识符"},
				},
			},
		},
	}
}

// CallTool is exported for testing. Production code uses callToolFn via handleToolCall.
func (s *MCPServer) CallTool(name string, args json.RawMessage) (any, error) {
	return s.callToolFn(name, args)
}

// beeCallTool dispatches to the named tool handler and returns the result (all 19 tools).
func (s *MCPServer) beeCallTool(name string, args json.RawMessage) (any, error) {
	switch name {
	case toolnames.ListWorkers:
		return s.toolListWorkers(args)
	case toolnames.GetWorker:
		return s.toolGetWorker(args)
	case toolnames.CreateWorker:
		return s.toolCreateWorker(args)
	case toolnames.UpdateWorker:
		return s.toolUpdateWorker(args)
	case toolnames.DeleteWorker:
		return s.toolDeleteWorker(args)
	case toolnames.CreateTask:
		return s.toolCreateTask(args)
	case toolnames.ListTasks:
		return s.toolListTasks(args)
	case toolnames.CancelTask:
		return s.toolCancelTask(args)
	case toolnames.MarkTaskComplete:
		return s.toolMarkTaskComplete(args)
	case toolnames.SendMessage:
		return s.toolSendMessage(args)
	case toolnames.ClearSession:
		return s.toolClearSession(args)
	case toolnames.GetWorkerStatus:
		return s.toolGetWorkerStatus(args)
	case toolnames.GetSystemOverview:
		return s.toolGetSystemOverview(args)
	case toolnames.ListBeeExecutions:
		return s.toolListBeeExecutions(args)
	case toolnames.SaveMemory:
		return s.toolSaveMemory(args)
	case toolnames.GetMemory:
		return s.toolGetMemory(args)
	case toolnames.DeleteMemory:
		return s.toolDeleteMemory(args)
	case toolnames.ListSessionContexts:
		return s.toolListSessionContexts(args)
	case toolnames.ClearWorkerSession:
		return s.toolClearWorkerSession(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// workerCallTool dispatches to worker-only tool handlers (5 tools).
func (s *MCPServer) workerCallTool(name string, args json.RawMessage) (any, error) {
	switch name {
	case toolnames.MarkTaskComplete:
		return s.toolMarkTaskComplete(args)
	case toolnames.SendMessage:
		return s.toolSendMessage(args)
	case toolnames.SaveMemory:
		return s.toolSaveMemory(args)
	case toolnames.GetMemory:
		return s.toolGetMemory(args)
	case toolnames.DeleteMemory:
		return s.toolDeleteMemory(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func (s *MCPServer) toolListWorkers(_ json.RawMessage) (any, error) {
	workers, err := s.workerStore.List()
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
		Name        string `json:"name"`
		Description string `json:"description"`
		Memory      string `json:"memory"`
		WorkDir     string `json:"work_dir"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	return s.manager.CreateWorker(params.Name, params.Description, params.Memory, params.WorkDir)
}

func (s *MCPServer) toolUpdateWorker(args json.RawMessage) (any, error) {
	var params struct {
		WorkerID    string  `json:"worker_id"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Memory      *string `json:"memory"`
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

	return s.workerStore.Update(w)
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

func (s *MCPServer) toolMarkTaskComplete(args json.RawMessage) (any, error) {
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
	task, err := s.taskStore.GetByID(ctx, params.TaskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if task.Type == model.TaskTypeScheduled && task.CronExpr != "" {
		reset, err := s.taskStore.CompleteScheduledTask(ctx, params.TaskID)
		if err != nil {
			return nil, fmt.Errorf("reset scheduled task: %w", err)
		}
		if !reset {
			return map[string]string{"task_id": params.TaskID, "status": "cancelled"}, nil
		}
		return map[string]string{"task_id": params.TaskID, "status": "pending"}, nil
	}
	if err := s.taskStore.UpdateStatus(ctx, params.TaskID, model.TaskStatusCompleted); err != nil {
		return nil, fmt.Errorf("mark task success: %w", err)
	}
	return map[string]string{"task_id": params.TaskID, "status": "completed"}, nil
}

func (s *MCPServer) toolSendMessage(args json.RawMessage) (any, error) {
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

	stored, err := s.messageStore.GetByID(context.Background(), params.MessageID)
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

	// Send text first if both content and media_path are provided
	if params.Content != "" {
		outbound := platform.OutboundMessage{ReplyTo: replyTo, Content: params.Content}
		if err := sender.Send(context.Background(), outbound); err != nil {
			return nil, fmt.Errorf("send text message: %w", err)
		}
	}

	// Send media if media_path is provided
	if params.MediaPath != "" {
		outbound := platform.OutboundMessage{ReplyTo: replyTo, MediaPath: params.MediaPath}
		if err := sender.Send(context.Background(), outbound); err != nil {
			return nil, fmt.Errorf("send media message: %w", err)
		}
	}

	return map[string]string{"status": "sent"}, nil
}

func (s *MCPServer) toolClearSession(args json.RawMessage) (any, error) {
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

	ctx := context.Background()

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
				"message":               fmt.Sprintf("此会话链接了 %d 位员工，清空将重置所有员工和 bee 的对话上下文。请确认后以 force=true 重新调用。", len(workers)),
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

	result := map[string]any{
		"worker_id":         worker.ID,
		"name":              worker.Name,
		"status":            string(worker.Status),
		"current_execution": nil,
	}

	// Get running execution for this worker and find associated task_id
	execs, err := s.executionStore.ListByWorkerID(worker.ID)
	if err == nil {
		for _, e := range execs {
			if e.Status == "running" {
				execInfo := map[string]any{
					"id":          e.ID,
					"task_id":     nil,
					"instruction": e.TriggerInput,
					"started_at":  e.StartedAt,
				}
				ctx := context.Background()
				task, terr := s.taskStore.GetTaskByExecutionID(ctx, e.ID)
				if terr == nil && task != nil {
					execInfo["task_id"] = task.ID
				}
				result["current_execution"] = execInfo
				break
			}
		}
	}

	// Count pending tasks
	ctx := context.Background()
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
