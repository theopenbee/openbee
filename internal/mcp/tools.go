package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/infra/utils"
	"github.com/theopenbee/openbee/internal/platform"
)

// CallTool is exported for testing only.
func (s *MCPServer) CallTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	return s.beeCallTool(ctx, name, args)
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

// checkWorkerScope enforces scope-based access control for worker tokens.
// Bee tokens (empty workerID in context) are always allowed.
// Worker tokens must have the required scope for tools listed in auth.ToolScopeMap.
// Tools not in ToolScopeMap are unaffected (existing access rules apply).
func (s *MCPServer) checkWorkerScope(ctx context.Context, toolName string) error {
	workerID, _ := ctx.Value(CtxWorkerIDKey).(string)
	if workerID == "" {
		return nil
	}
	requiredScope, ok := auth.ToolScopeMap[toolName]
	if !ok {
		return nil
	}
	scopes, _ := ctx.Value(CtxScopesKey).([]string)
	if slices.Contains(scopes, requiredScope) {
		return nil
	}
	return fmt.Errorf("permission denied: scope %s required", requiredScope)
}

// beeCallTool dispatches to the named tool handler and returns the result.
func (s *MCPServer) beeCallTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	if err := s.checkWorkerScope(ctx, name); err != nil {
		return nil, err
	}
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
		return s.toolCreateTask(ctx, args)
	case utils.ListTasks:
		return s.toolListTasks(ctx, args)
	case utils.CancelTask:
		return s.toolCancelTask(ctx, args)
	case utils.SendMessage:
		return s.toolSendMessage(ctx, args)
	case utils.ClearSession:
		return s.toolClearSession(ctx, args)
	case utils.GetWorkerStatus:
		return s.toolGetWorkerStatus(ctx, args)
	case utils.GetSystemOverview:
		return s.toolGetSystemOverview(ctx)
	case utils.ListBeeExecutions:
		return s.toolListBeeExecutions(args)
	case utils.SaveMemory:
		return s.toolSaveMemory(args)
	case utils.GetMemory:
		return s.toolGetMemory(args)
	case utils.DeleteMemory:
		return s.toolDeleteMemory(args)
	case utils.ListSessionContexts:
		return s.toolListSessionContexts(ctx, args)
	case utils.ClearWorkerSession:
		return s.toolClearWorkerSession(ctx, args)
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
		recursive := params.Recursive == nil || *params.Recursive
		if recursive {
			workers, err = s.listWorkersRecursive(params.DepartmentID)
		} else {
			deptID, resolveErr := s.resolveDepartmentID(params.DepartmentID)
			if resolveErr != nil {
				return nil, resolveErr
			}
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
		Name             string `json:"name"`
		Description      string `json:"description"`
		Memory           string `json:"memory"`
		WorkDir          string `json:"work_dir"`
		DepartmentIDs    string `json:"department_ids"`
		PermissionScopes string `json:"permission_scopes"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	w, err := s.manager.CreateWorker(params.Name, params.Description, params.Memory, params.WorkDir, params.PermissionScopes)
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
		WorkerID         string  `json:"worker_id"`
		Name             *string `json:"name"`
		Description      *string `json:"description"`
		Memory           *string `json:"memory"`
		DepartmentIDs    *string `json:"department_ids"`
		PermissionScopes *string `json:"permission_scopes"`
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
	fieldsChanged := params.Name != nil || params.Description != nil || params.Memory != nil || params.PermissionScopes != nil
	if params.Name != nil {
		w.Name = *params.Name
	}
	if params.Description != nil {
		w.Description = *params.Description
	}
	if params.Memory != nil {
		w.Memory = *params.Memory
	}
	if params.PermissionScopes != nil {
		w.PermissionScopes = *params.PermissionScopes
	}
	if fieldsChanged {
		w, err = s.workerStore.Update(w)
		if err != nil {
			return nil, err
		}
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

func (s *MCPServer) toolCreateTask(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		MessageID   string `json:"message_id"`
		WorkerID    string `json:"worker_id"`
		Instruction string `json:"instruction"`
		Type        string `json:"type"`
		ScheduledAt *int64 `json:"scheduled_at"`
		CronExpr    string `json:"cron_expr"`
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
			id, createErr := s.taskStore.Create(ctx, task)
			if createErr != nil {
				return nil, fmt.Errorf("create cancelled task: %w", createErr)
			}
			return map[string]string{"task_id": id, "status": "cancelled", "reason": "invalid cron_expr: " + err.Error()}, nil
		}
		next := sched.Next(time.Now()).UnixMilli()
		nextRunAt = &next
	}

	task := model.Task{
		MessageID:   params.MessageID,
		WorkerID:    params.WorkerID,
		Instruction: params.Instruction,
		Type:        params.Type,
		Status:      model.TaskStatusPending,
		ScheduledAt: params.ScheduledAt,
		CronExpr:    params.CronExpr,
		NextRunAt:   nextRunAt,
		CreatedAt:   nowMS,
		UpdatedAt:   nowMS,
	}

	id, err := s.taskStore.Create(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return map[string]string{"task_id": id, "status": "pending"}, nil
}

func (s *MCPServer) toolListTasks(ctx context.Context, args json.RawMessage) (any, error) {
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
	tasks, err := s.taskStore.List(ctx, store.TaskFilter{
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

func (s *MCPServer) toolCancelTask(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}

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
		seenWorkers := make(map[string]struct{})
		for _, a := range agents {
			if a.AgentType == "worker" {
				if _, exists := seenWorkers[a.AgentID]; exists {
					continue
				}
				seenWorkers[a.AgentID] = struct{}{}
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

func (s *MCPServer) toolGetWorkerStatus(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		WorkerID string `json:"worker_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
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

func (s *MCPServer) toolGetSystemOverview(ctx context.Context) (any, error) {
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
		return nil, fmt.Errorf("invalid args: %w", err)
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
		return nil, fmt.Errorf("invalid args: %w", err)
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
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if p.Scope == "" || p.Key == "" {
		return nil, fmt.Errorf("scope and key are required")
	}
	if err := s.memoryStore.Delete(p.Scope, p.Key); err != nil {
		return nil, fmt.Errorf("failed to delete memory: %w", err)
	}
	return map[string]string{"status": "deleted"}, nil
}

func (s *MCPServer) toolListSessionContexts(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionKey string `json:"session_key"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.SessionKey == "" {
		return nil, fmt.Errorf("session_key is required")
	}
	agents, err := s.sessionStore.ListSessionContexts(ctx, params.SessionKey)
	if err != nil {
		return nil, fmt.Errorf("list session contexts: %w", err)
	}
	return agents, nil
}

func (s *MCPServer) toolClearWorkerSession(ctx context.Context, args json.RawMessage) (any, error) {
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
	toResolve := []string{params.ID}
	if params.ParentID != nil {
		toResolve = append(toResolve, *params.ParentID)
	}
	resolvedIDs, err := s.resolveDepartmentIDs(toResolve)
	if err != nil {
		return nil, err
	}
	deptID := resolvedIDs[0]
	d, err := s.departmentStore.GetByID(deptID)
	if err != nil {
		return nil, fmt.Errorf("get department: %w", err)
	}
	fieldsChanged := params.Name != nil || params.SortOrder != nil || params.ParentID != nil
	if params.Name != nil {
		d.Name = *params.Name
	}
	if params.SortOrder != nil {
		d.SortOrder = *params.SortOrder
	}
	if params.ParentID != nil {
		resolvedParentID := resolvedIDs[1]
		if err := s.departmentStore.CheckCircularReference(d.ID, resolvedParentID); err != nil {
			return nil, err
		}
		d.ParentID = &resolvedParentID
	}
	if !fieldsChanged {
		return d, nil
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
	ids, err := s.resolveDepartmentIDs([]string{idOrName})
	if err != nil {
		return "", err
	}
	return ids[0], nil
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
	return resolveDeptIDsFromList(all, idOrNames)
}

// resolveDeptIDsFromList resolves ID-or-name strings using an already-loaded department list,
// avoiding an extra DB query when the caller already has all departments in memory.
func resolveDeptIDsFromList(all []model.Department, idOrNames []string) ([]string, error) {
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
		parts = append(parts, cur.Name)
		if cur.ParentID == nil {
			break
		}
		parent, ok := deptMap[*cur.ParentID]
		if !ok {
			break
		}
		cur = parent
	}
	// Reverse: we collected child→root, want root→child.
	slices.Reverse(parts)
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

func (s *MCPServer) listWorkersRecursive(idOrName string) ([]model.Worker, error) {
	all, err := s.departmentStore.ListAll()
	if err != nil {
		return nil, fmt.Errorf("list departments: %w", err)
	}
	resolvedIDs, err := resolveDeptIDsFromList(all, []string{idOrName})
	if err != nil {
		return nil, err
	}
	deptIDs := collectDescendantIDs(all, resolvedIDs[0])
	workerIDs, err := s.departmentStore.GetWorkerIDsForDepartments(deptIDs)
	if err != nil {
		return nil, fmt.Errorf("get department workers: %w", err)
	}
	return s.workerStore.GetByIDs(workerIDs)
}
