package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/theopenbee/openbee/internal/domain/worker"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/infra/utils"
	"github.com/theopenbee/openbee/internal/platform"
)

// CallTool is exported for testing only.
func (s *Server) CallTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	return s.beeCallTool(ctx, name, args)
}

// workerDisplayName returns the worker's configured name, falling back to the raw ID.
func (s *Server) workerDisplayName(workerID string) string {
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

// checkWorkerScope enforces per-tool scope restrictions for worker tokens.
// Bee tokens carry no workerID and are always fully trusted — only worker tokens are scope-restricted.
func (s *Server) checkWorkerScope(ctx context.Context, toolName string) error {
	workerID, _ := ctx.Value(CtxWorkerIDKey).(string)
	if workerID == "" {
		return nil
	}
	requiredScope, ok := auth.ScopeForTool(toolName)
	if !ok {
		return nil
	}
	scopes, _ := ctx.Value(CtxScopesKey).([]string)
	if slices.Contains(scopes, requiredScope) {
		return nil
	}
	return fmt.Errorf("permission denied: scope %s required", requiredScope)
}

func (s *Server) beeCallTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	if err := s.checkWorkerScope(ctx, name); err != nil {
		return nil, err
	}
	switch name {
	case utils.ListWorkers:
		return s.toolListWorkers(ctx, args)
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
	case utils.SaveConstraint:
		return s.toolSaveConstraint(args)
	case utils.GetConstraint:
		return s.toolGetConstraint(args)
	case utils.DeleteConstraint:
		return s.toolDeleteConstraint(args)
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
	case utils.ListMessages:
		return s.toolListMessages(ctx, args)
	case utils.ListOutboundMessages:
		return s.toolListOutboundMessages(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

const (
	ClearReasonActiveTasks     = "active_tasks"
	ClearReasonMultipleWorkers = "multiple_workers"
	linearPlatformID           = "linear"
)

type workerBrief struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Engine      string `json:"engine"`
}

type ActiveTaskSummary struct {
	TaskID      string `json:"task_id"`
	Instruction string `json:"instruction"`
	Status      string `json:"status"`
}

type LinkedWorkerSummary struct {
	WorkerID string `json:"worker_id"`
	Name     string `json:"name"`
}

func (s *Server) toolListWorkers(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		DepartmentID string `json:"department_id"`
		Recursive    *bool  `json:"recursive"`
		Name         string `json:"name"`
		ID           string `json:"id"`
		Page         int    `json:"page"`
		PageSize     int    `json:"page_size"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}

	page, pageSize, offset := normalizePage(params.Page, params.PageSize, 200)

	filter := store.WorkerFilter{
		Name: params.Name,
		ID:   params.ID,
	}

	if params.DepartmentID != "" {
		recursive := params.Recursive == nil || *params.Recursive
		var workerIDs []string
		var err error
		if recursive {
			workerIDs, err = s.listWorkersRecursive(params.DepartmentID)
		} else {
			deptID, resolveErr := s.resolveDepartmentID(params.DepartmentID)
			if resolveErr != nil {
				return nil, resolveErr
			}
			workerIDs, err = s.departmentStore.GetWorkerIDsForDepartments([]string{deptID})
		}
		if err != nil {
			return nil, fmt.Errorf("list workers: %w", err)
		}
		filter.WorkerIDs = workerIDs
	}

	workers, total, err := s.workerStore.ListFiltered(ctx, filter, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}

	briefs := make([]workerBrief, 0, len(workers))
	for _, w := range workers {
		briefs = append(briefs, workerBrief{ID: w.ID, Name: w.Name, Description: w.Description, Engine: w.Engine})
	}

	return pagedResult(briefs, total, page, pageSize), nil
}

func (s *Server) toolGetWorker(args json.RawMessage) (any, error) {
	var params struct {
		WorkerID string `json:"worker_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}
	type workerResult struct {
		w   model.Worker
		err error
	}
	type deptsResult struct {
		depts []model.Department
		err   error
	}
	wCh := make(chan workerResult, 1)
	dCh := make(chan deptsResult, 1)
	go func() {
		w, err := s.workerStore.GetByID(params.WorkerID)
		wCh <- workerResult{w, err}
	}()
	go func() {
		depts, err := s.departmentStore.GetWorkerDepartments(params.WorkerID)
		dCh <- deptsResult{depts, err}
	}()
	wr, dr := <-wCh, <-dCh
	if wr.err != nil {
		return nil, fmt.Errorf("worker not found: %w", wr.err)
	}
	if dr.err != nil {
		return nil, fmt.Errorf("get worker departments: %w", dr.err)
	}
	return model.WorkerWithDepartments{Worker: wr.w, Departments: model.ToDepartmentBriefs(dr.depts)}, nil
}

func (s *Server) toolCreateWorker(args json.RawMessage) (any, error) {
	var params struct {
		Name             string `json:"name"`
		Description      string `json:"description"`
		Constraints      string `json:"constraints"`
		WorkDir          string `json:"work_dir"`
		Engine           string `json:"engine"`
		DepartmentIDs    string `json:"department_ids"`
		PermissionScopes string `json:"permission_scopes"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := s.manager.ValidateEngine(params.Engine); err != nil {
		return nil, err
	}
	if err := auth.ValidatePermissionScopes(params.PermissionScopes); err != nil {
		return nil, err
	}
	w, err := s.manager.CreateWorker(worker.CreateWorkerParams{
		Name:             params.Name,
		Description:      params.Description,
		Constraints:      params.Constraints,
		WorkDir:          params.WorkDir,
		Engine:           params.Engine,
		PermissionScopes: params.PermissionScopes,
	})
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

func (s *Server) toolUpdateWorker(args json.RawMessage) (any, error) {
	var params struct {
		WorkerID      string  `json:"worker_id"`
		DepartmentIDs *string `json:"department_ids"`
		worker.UpdateWorkerParams
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}
	w, err := s.manager.UpdateWorker(params.WorkerID, params.UpdateWorkerParams)
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

func (s *Server) toolDeleteWorker(args json.RawMessage) (any, error) {
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

func (s *Server) toolCreateTask(ctx context.Context, args json.RawMessage) (any, error) {
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
	case model.TaskTypeImmediate, model.TaskTypeCountdown, model.TaskTypeScheduled:
	default:
		return nil, fmt.Errorf("type must be %s, %s, or %s",
			model.TaskTypeImmediate, model.TaskTypeCountdown, model.TaskTypeScheduled)
	}

	nowMS := time.Now().UnixMilli()

	switch params.Type {
	case model.TaskTypeCountdown:
		if params.ScheduledAt == nil {
			return nil, fmt.Errorf("scheduled_at is required for countdown tasks")
		}
		if *params.ScheduledAt < nowMS+5000 {
			return nil, fmt.Errorf("scheduled_at must be at least 5 seconds in the future")
		}
	case model.TaskTypeScheduled:
		if params.CronExpr == "" {
			return nil, fmt.Errorf("cron_expr is required for scheduled tasks")
		}
	}

	var nextRunAt *int64
	if params.Type == model.TaskTypeScheduled {
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

type taskWithExecutions struct {
	model.Task
	Executions []model.WorkerExecution `json:"executions"`
}

func (s *Server) toolListTasks(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		TaskID         string `json:"task_id"`
		MessageID      string `json:"message_id"`
		SessionKey     string `json:"session_key"`
		WorkerID       string `json:"worker_id"`
		Status         string `json:"status"`
		Type           string `json:"type"`
		Page           int    `json:"page"`
		PageSize       int    `json:"page_size"`
		ExecutionLimit *int   `json:"execution_limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.TaskID != "" && (params.MessageID != "" || params.SessionKey != "" || params.WorkerID != "") {
		return nil, fmt.Errorf("task_id cannot be combined with message_id, session_key, or worker_id")
	}
	if params.MessageID != "" && params.SessionKey != "" {
		return nil, fmt.Errorf("message_id and session_key are mutually exclusive")
	}
	if params.TaskID == "" && params.MessageID == "" && params.SessionKey == "" && params.WorkerID == "" {
		return nil, fmt.Errorf("at least one of task_id, message_id, session_key, or worker_id is required")
	}

	page, pageSize, offset := normalizePage(params.Page, params.PageSize, maxTaskPageSize)

	filter := store.TaskFilter{
		TaskID:     params.TaskID,
		MessageID:  params.MessageID,
		SessionKey: params.SessionKey,
		WorkerID:   params.WorkerID,
		Status:     params.Status,
		Type:       params.Type,
		Limit:      pageSize,
		Offset:     offset,
	}
	var (
		total int
		tasks []model.Task
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		t, err := s.taskStore.CountTasks(gctx, filter)
		if err != nil {
			return fmt.Errorf("count tasks: %w", err)
		}
		total = t
		return nil
	})
	g.Go(func() error {
		ts, err := s.taskStore.List(gctx, filter)
		if err != nil {
			return fmt.Errorf("list tasks: %w", err)
		}
		tasks = ts
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if tasks == nil {
		tasks = []model.Task{}
	}
	executionLimit, err := normalizeTaskExecutionLimit(params.ExecutionLimit, len(tasks))
	if err != nil {
		return nil, err
	}
	taskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		taskIDs = append(taskIDs, t.ID)
	}
	execsByTask, err := s.executionStore.ListByTaskIDs(ctx, taskIDs, executionLimit)
	if err != nil {
		return nil, fmt.Errorf("list executions for tasks: %w", err)
	}
	out := make([]taskWithExecutions, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, taskWithExecutions{Task: t, Executions: execsByTask[t.ID]})
	}
	return pagedResult(out, total, page, pageSize), nil
}

func (s *Server) finalizeCancelledExecution(ctx context.Context, executionID string) {
	if _, err := s.executionStore.MarkAbandoned(ctx, executionID, "cancelled by user"); err != nil {
		log.Error("finalize cancelled execution", zap.String("executionID", executionID), zap.Error(err))
	}
}

func (s *Server) toolCancelTask(ctx context.Context, args json.RawMessage) (any, error) {
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
	running, err := s.executionStore.GetRunningByTaskID(ctx, params.TaskID)
	if err != nil {
		return nil, fmt.Errorf("get running execution: %w", err)
	}
	if running != nil {
		var stopErr error
		if s.execStopper != nil {
			stopErr = s.execStopper.StopExecution(running.ID)
		}
		if stopErr != nil {
			// Process not active in this server (already exited, never started,
			// or different instance). The execution row may be a stuck running
			// orphan — force-finalize so future busy checks don't trip on it.
			log.Debug("stop execution: process not active",
				zap.String("op", "cancel_task"),
				zap.String("executionID", running.ID),
				zap.Error(stopErr))
			s.finalizeCancelledExecution(ctx, running.ID)
		}
		// On stopErr == nil the process was alive; monitorExecution will
		// finalize the row when its output channel closes.
	}

	if err := s.taskCanceller.CancelTask(ctx, params.TaskID); err != nil {
		return nil, fmt.Errorf("cancel task: %w", err)
	}
	return map[string]string{"task_id": params.TaskID, "status": "cancelled"}, nil
}

func (s *Server) toolSendMessage(ctx context.Context, args json.RawMessage) (any, error) {
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

	if stored.Platform == linearPlatformID && params.Content != "" && params.MediaPath != "" {
		outbound := base
		outbound.Content = params.Content
		outbound.MediaPath = params.MediaPath
		if err := sender.Send(ctx, outbound); err != nil {
			return nil, fmt.Errorf("send linear message: %w", err)
		}
		return map[string]string{"status": "sent"}, nil
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

func (s *Server) toolClearSession(ctx context.Context, args json.RawMessage) (any, error) {
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

	var tasksToStop []model.Task
	if !params.Force {
		activeTasks, err := s.taskStore.ListBySessionKey(ctx, params.SessionKey,
			model.TaskStatusActive, model.TaskTypeImmediate)
		if err != nil {
			return nil, fmt.Errorf("list active tasks: %w", err)
		}
		if len(activeTasks) > 0 {
			summaries := make([]ActiveTaskSummary, 0, len(activeTasks))
			for _, t := range activeTasks {
				summaries = append(summaries, ActiveTaskSummary{
					TaskID:      t.ID,
					Instruction: t.Instruction,
					Status:      t.Status,
				})
			}
			return map[string]any{
				"requires_confirmation": true,
				"reason":                ClearReasonActiveTasks,
				"running_tasks":         summaries,
				"message":               fmt.Sprintf(i18n.M.Runtime.RPC.ClearSessionTasksConfirm, len(activeTasks)),
			}, nil
		}

		agents, err := s.sessionStore.ListSessionContexts(ctx, params.SessionKey)
		if err != nil {
			return nil, fmt.Errorf("list session contexts: %w", err)
		}
		var workers []LinkedWorkerSummary
		seenWorkers := make(map[string]struct{})
		for _, a := range agents {
			if a.AgentType == store.WorkerAgentType {
				if _, exists := seenWorkers[a.AgentID]; exists {
					continue
				}
				seenWorkers[a.AgentID] = struct{}{}
				workers = append(workers, LinkedWorkerSummary{WorkerID: a.AgentID, Name: a.Name})
			}
		}
		if len(workers) > 1 {
			return map[string]any{
				"requires_confirmation": true,
				"reason":                ClearReasonMultipleWorkers,
				"worker_count":          len(workers),
				"linked_workers":        workers,
				"message":               fmt.Sprintf(i18n.M.Runtime.RPC.ClearSessionConfirm, len(workers)),
			}, nil
		}
	} else {
		var err error
		tasksToStop, err = s.taskStore.ListBySessionKey(ctx, params.SessionKey, model.TaskStatusRunning, model.TaskTypeImmediate)
		if err != nil {
			return nil, fmt.Errorf("list running tasks: %w", err)
		}
	}

	// Stop processes before cancelling DB records so workers don't pick up new work after cancellation.
	execIDs := utils.RunningExecIDsForTasks(ctx, log, s.executionStore, tasksToStop, "clear_session")
	for _, t := range tasksToStop {
		execID := execIDs[t.ID]
		if execID == "" {
			continue
		}
		if err := s.execStopper.StopExecution(execID); err != nil {
			log.Debug("stop execution: process not active",
				zap.String("op", "clear_session"),
				zap.String("executionID", execID),
				zap.Error(err))
			s.finalizeCancelledExecution(ctx, execID)
		}
	}

	cancelled, err := s.taskStore.CancelBySessionKey(ctx, params.SessionKey, model.TaskTypeImmediate)
	if err != nil {
		return nil, fmt.Errorf("cancel tasks: %w", err)
	}

	if s.sessionClearer != nil {
		s.sessionClearer.ClearSession(params.SessionKey)
	}

	return map[string]any{
		"cancelled_tasks": cancelled,
		"cleared":         true,
	}, nil
}

func (s *Server) toolGetWorkerStatus(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		WorkerID string `json:"worker_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if p.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}

	type workerRes struct {
		w   model.Worker
		err error
	}
	type execRes struct {
		exec *model.WorkerExecution
		err  error
	}
	type pendingRes struct {
		count int
		err   error
	}
	wCh := make(chan workerRes, 1)
	eCh := make(chan execRes, 1)
	pCh := make(chan pendingRes, 1)
	go func() {
		w, err := s.workerStore.GetByID(p.WorkerID)
		wCh <- workerRes{w, err}
	}()
	go func() {
		exec, err := s.executionStore.GetRunningByWorkerID(p.WorkerID)
		eCh <- execRes{exec, err}
	}()
	go func() {
		count, err := s.taskStore.CountPendingByWorkerID(ctx, p.WorkerID)
		pCh <- pendingRes{count, err}
	}()
	wr, er, pr := <-wCh, <-eCh, <-pCh
	if wr.err != nil {
		return nil, fmt.Errorf("worker not found: %w", wr.err)
	}
	pendingCount := pr.count

	result := map[string]any{
		"worker_id":           wr.w.ID,
		"name":                wr.w.Name,
		"status":              string(wr.w.Status),
		"current_execution":   nil,
		"pending_tasks_count": pendingCount,
	}

	if er.err == nil && er.exec != nil {
		result["current_execution"] = map[string]any{
			"id":          er.exec.ID,
			"task_id":     er.exec.TaskID,
			"instruction": er.exec.TriggerInput,
			"started_at":  er.exec.StartedAt,
		}
	}

	return result, nil
}

func (s *Server) toolGetSystemOverview(ctx context.Context) (any, error) {
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
			"idle":    workerCounts[string(model.WorkerStatusIdle)],
			"working": workerCounts[string(model.WorkerStatusWorking)],
			"error":   workerCounts[string(model.WorkerStatusError)],
		},
		"tasks": map[string]any{
			"pending":          taskCounts[model.TaskStatusPending],
			"running":          taskCounts[model.TaskStatusRunning],
			"completed":        taskCounts[model.TaskStatusCompleted],
			"failed":           taskCounts[model.TaskStatusFailed],
			"cancelled":        taskCounts[model.TaskStatusCancelled],
			"scheduled_active": scheduledActive,
		},
		"recent_executions": recentList,
	}, nil
}

func (s *Server) toolSaveConstraint(args json.RawMessage) (any, error) {
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
	if err := s.constraintStore.Save(p.Scope, p.Key, p.Value); err != nil {
		return nil, fmt.Errorf("failed to save constraint: %w", err)
	}
	return map[string]string{"status": "saved"}, nil
}

func (s *Server) toolGetConstraint(args json.RawMessage) (any, error) {
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
		c, err := s.constraintStore.Get(p.Scope, p.Key)
		if err != nil {
			return nil, fmt.Errorf("failed to get constraint: %w", err)
		}
		if c == nil {
			return map[string]string{"status": "not_found"}, nil
		}
		return c, nil
	}
	constraints, err := s.constraintStore.ListByScope(p.Scope, 50)
	if err != nil {
		return nil, fmt.Errorf("failed to list constraints: %w", err)
	}
	return constraints, nil
}

func (s *Server) toolDeleteConstraint(args json.RawMessage) (any, error) {
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
	if err := s.constraintStore.Delete(p.Scope, p.Key); err != nil {
		return nil, fmt.Errorf("failed to delete constraint: %w", err)
	}
	return map[string]string{"status": "deleted"}, nil
}

func (s *Server) toolListSessionContexts(ctx context.Context, args json.RawMessage) (any, error) {
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

func (s *Server) toolClearWorkerSession(ctx context.Context, args json.RawMessage) (any, error) {
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

	workerName := s.workerDisplayName(params.WorkerID)

	if err := s.sessionStore.DeleteWorkerSessionContext(ctx, params.SessionKey, params.WorkerID); err != nil {
		return nil, fmt.Errorf("delete worker session context: %w", err)
	}

	return map[string]any{
		"cleared":     true,
		"worker_id":   params.WorkerID,
		"worker_name": workerName,
	}, nil
}

type deptTreeBrief struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	SortOrder int             `json:"sort_order"`
	Children  []deptTreeBrief `json:"children"`
}

func toDeptTreeBriefs(nodes []model.DepartmentTree) []deptTreeBrief {
	result := make([]deptTreeBrief, 0, len(nodes))
	for _, n := range nodes {
		result = append(result, deptTreeBrief{
			ID:        n.ID,
			Name:      n.Name,
			SortOrder: n.SortOrder,
			Children:  toDeptTreeBriefs(n.Children),
		})
	}
	return result
}

func (s *Server) toolListDepartments(_ json.RawMessage) (any, error) {
	all, err := s.departmentStore.ListAll()
	if err != nil {
		return nil, fmt.Errorf("list departments: %w", err)
	}
	tree := s.departmentStore.BuildTree(all)
	return toDeptTreeBriefs(tree), nil
}

func (s *Server) toolGetDepartment(args json.RawMessage) (any, error) {
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

func (s *Server) toolCreateDepartment(args json.RawMessage) (any, error) {
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

func (s *Server) toolUpdateDepartment(args json.RawMessage) (any, error) {
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

func (s *Server) toolDeleteDepartment(args json.RawMessage) (any, error) {
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

func (s *Server) resolveDepartmentID(idOrName string) (string, error) {
	ids, err := s.resolveDepartmentIDs([]string{idOrName})
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

// resolveDepartmentIDs resolves a slice of ID-or-name strings to department IDs with a single
// ListAll call to avoid per-element DB queries.
func (s *Server) resolveDepartmentIDs(idOrNames []string) ([]string, error) {
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
func (s *Server) applyWorkerDepartments(workerID, deptIDsParam string) error {
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

func (s *Server) listWorkersRecursive(idOrName string) ([]string, error) {
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
	return workerIDs, nil
}

const (
	maxTaskPageSize           = 100
	defaultTaskExecutionLimit = 10
	maxTaskExecutionLimit     = 100
)

func normalizeTaskExecutionLimit(raw *int, matchedTasks int) (int, error) {
	if raw == nil {
		return defaultTaskExecutionLimit, nil
	}
	if *raw == 0 {
		if matchedTasks != 1 {
			return 0, fmt.Errorf("execution_limit=0 requires exactly one matching task; use task_id to select one task")
		}
		return 0, nil
	}
	if *raw < 0 {
		return 0, fmt.Errorf("execution_limit must be >= 0")
	}
	if *raw > maxTaskExecutionLimit {
		return maxTaskExecutionLimit, nil
	}
	return *raw, nil
}

func pagedResult(items any, total, page, pageSize int) map[string]any {
	return map[string]any{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}
}

// normalizePage clamps page/pageSize to valid ranges and returns the offset.
func normalizePage(page, pageSize, maxPageSize int) (int, int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize, (page - 1) * pageSize
}

func (s *Server) toolListMessages(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionKey     string `json:"session_key"`
		Platform       string `json:"platform"`
		Status         string `json:"status"`
		ReceivedAtFrom int64  `json:"received_at_from"`
		ReceivedAtTo   int64  `json:"received_at_to"`
		Page           int    `json:"page"`
		PageSize       int    `json:"page_size"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	var offset int
	params.Page, params.PageSize, offset = normalizePage(params.Page, params.PageSize, 100)
	msgs, total, err := s.messageStore.ListFiltered(ctx, store.MessageFilter{
		SessionKey:     params.SessionKey,
		Platform:       params.Platform,
		Status:         params.Status,
		ReceivedAtFrom: params.ReceivedAtFrom,
		ReceivedAtTo:   params.ReceivedAtTo,
	}, params.PageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	return pagedResult(msgs, total, params.Page, params.PageSize), nil
}

func (s *Server) toolListOutboundMessages(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionKey string `json:"session_key"`
		Platform   string `json:"platform"`
		Status     string `json:"status"`
		SourceType string `json:"source_type"`
		SourceID   string `json:"source_id"`
		SentAtFrom int64  `json:"sent_at_from"`
		SentAtTo   int64  `json:"sent_at_to"`
		Page       int    `json:"page"`
		PageSize   int    `json:"page_size"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	var offset int
	params.Page, params.PageSize, offset = normalizePage(params.Page, params.PageSize, 100)
	msgs, total, err := s.outboundMessageStore.ListFiltered(ctx, store.OutboundMessageFilter{
		SessionKey: params.SessionKey,
		Platform:   params.Platform,
		Status:     params.Status,
		SourceType: params.SourceType,
		SourceID:   params.SourceID,
		SentAtFrom: params.SentAtFrom,
		SentAtTo:   params.SentAtTo,
	}, params.PageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("list outbound messages: %w", err)
	}
	return pagedResult(msgs, total, params.Page, params.PageSize), nil
}

