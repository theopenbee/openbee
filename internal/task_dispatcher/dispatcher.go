package task_dispatcher

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/robobee/core/internal/model"
	"github.com/robobee/core/internal/platform"
)

const (
	pollInterval = 2 * time.Second
	pollTimeout  = 30 * time.Minute
)

// ExecutionManager manages worker executions.
type ExecutionManager interface {
	ExecuteWorker(ctx context.Context, workerID, input, sessionID string) (model.WorkerExecution, error)
}

// ExecutionQuerier retrieves execution state by ID.
type ExecutionQuerier interface {
	GetByID(id string) (model.WorkerExecution, error)
}

// TaskStore is the subset of store.TaskStore used by the TaskDispatcher.
type TaskStore interface {
	SetExecution(ctx context.Context, taskID, executionID, status string) error
}

// SessionStore is the subset of store.SessionStore used by the TaskDispatcher.
type SessionStore interface {
	GetSessionContext(ctx context.Context, sessionKey, agentID string) (string, error)
	UpsertSessionContext(ctx context.Context, sessionKey, agentID, sessionID string) error
	ClearSessionContexts(ctx context.Context, sessionKey string) error
}

type queueState struct {
	executing    bool
	pendingTasks []DispatchTask
	lastReplyTo  platform.InboundMessage
}

type internalResult struct {
	queueKey string
	task     DispatchTask
}

// TaskDispatcher serializes worker executions per (SessionKey, WorkerID).
type TaskDispatcher struct {
	ctx          context.Context        // 由 Run 注入的生命周期上下文
	manager      ExecutionManager       // 启动 worker 执行
	taskStore    TaskStore              // 持久化 task 与 execution 的关联状态
	sessionStore SessionStore           // 管理会话上下文的读写与清理
	execQuerier  ExecutionQuerier       // 按 ID 查询 execution 状态
	in           <-chan DispatchTask    // 入站任务通道
	results      chan internalResult    // 内部完成信号通道，用于驱动队列调度
	queues       map[string]*queueState // 按 sessionKey|workerID 分组的串行队列
	clearCh      chan string            // 接收需要清理的 sessionKey 信号
}

// New constructs a TaskDispatcher.
func New(manager ExecutionManager, taskStore TaskStore, sessionStore SessionStore, execQuerier ExecutionQuerier, in <-chan DispatchTask) *TaskDispatcher {
	return &TaskDispatcher{
		manager:      manager,
		taskStore:    taskStore,
		sessionStore: sessionStore,
		execQuerier:  execQuerier,
		in:           in,
		results:      make(chan internalResult, 64),
		queues:       make(map[string]*queueState),
		clearCh:      make(chan string, 8),
	}
}

// Run processes tasks until ctx is cancelled. Call in a goroutine.
func (d *TaskDispatcher) Run(ctx context.Context) {
	d.ctx = ctx
	for {
		select {
		case task, ok := <-d.in:
			if !ok {
				return
			}
			d.handleInbound(task)
		case res := <-d.results:
			d.handleResult(res)
		case sessionKey := <-d.clearCh:
			d.clearQueues(sessionKey)
		case <-ctx.Done():
			return
		}
	}
}

func queueKey(sessionKey, workerID string) string {
	return sessionKey + "|" + workerID
}

func (d *TaskDispatcher) handleInbound(task DispatchTask) {
	key := queueKey(task.SessionKey, task.WorkerID)
	state, ok := d.queues[key]
	if !ok {
		state = &queueState{}
		d.queues[key] = state
	}

	replyTo := task.ReplyTo

	if !state.executing {
		state.executing = true
		state.lastReplyTo = replyTo
		go d.executeAsync(d.ctx, key, task, replyTo)
	} else {
		state.pendingTasks = append(state.pendingTasks, task)
		state.lastReplyTo = replyTo
	}
}

// ClearSession removes all queued tasks for the given session and clears session contexts.
// Safe to call from any goroutine — uses a buffered channel to signal the Run loop.
func (d *TaskDispatcher) ClearSession(sessionKey string) {
	select {
	case d.clearCh <- sessionKey:
	default:
		slog.Warn("clearCh full, dropping clear", "component", "taskdispatcher", "sessionKey", sessionKey)
	}
}

func (d *TaskDispatcher) clearQueues(sessionKey string) {
	prefix := sessionKey + "|"
	for key := range d.queues {
		if strings.HasPrefix(key, prefix) {
			delete(d.queues, key)
		}
	}
	if err := d.sessionStore.ClearSessionContexts(d.ctx, sessionKey); err != nil {
		slog.Error("clear session contexts", "component", "taskdispatcher", "sessionKey", sessionKey, "error", err)
	}
}

// buildInstruction prepends task metadata to the instruction so workers
// can call mark_task_success/failed and send_message via MCP.
func buildInstruction(task DispatchTask) string {
	if task.TaskID == "" {
		return task.Instruction
	}
	return fmt.Sprintf("[系统元数据] task_id=%s message_id=%s\n\n%s",
		task.TaskID, task.MessageID, task.Instruction)
}

func (d *TaskDispatcher) executeAsync(ctx context.Context, key string, task DispatchTask, replyTo platform.InboundMessage) {
	var exec model.WorkerExecution
	var err error

	instruction := buildInstruction(task)

	if task.TaskType == model.TaskTypeImmediate {
		sessionID, sessErr := d.sessionStore.GetSessionContext(ctx, task.SessionKey, task.WorkerID)
		if sessErr != nil {
			slog.Error("get session context", "component", "taskdispatcher", "error", sessErr)
		}
		if sessionID != "" {
			slog.Info("resuming session", "component", "taskdispatcher", "sessionID", sessionID, "taskID", task.TaskID)
			exec, err = d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, sessionID)
			if err != nil {
				slog.Error("resume error, falling back to fresh", "component", "taskdispatcher", "error", err)
				if clearErr := d.sessionStore.ClearSessionContexts(ctx, task.SessionKey); clearErr != nil {
					slog.Error("clear stale session contexts", "component", "taskdispatcher", "sessionKey", task.SessionKey, "error", clearErr)
				}
				exec, err = d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, "")
			}
			goto execStarted
		}
	}

	slog.Info("executing worker", "component", "taskdispatcher", "workerID", task.WorkerID, "taskID", task.TaskID)
	exec, err = d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, "")

execStarted:
	if err != nil {
		slog.Error("execute error", "component", "taskdispatcher", "error", err)
		select {
		case d.results <- internalResult{queueKey: key, task: task}:
		case <-ctx.Done():
		}
		return
	}

	if task.TaskID != "" {
		d.taskStore.SetExecution(ctx, task.TaskID, exec.ID, model.TaskStatusRunning) //nolint:errcheck
	}
	d.waitForResult(ctx, exec.ID, task.TaskID, task.SessionKey, task.WorkerID)
	select {
	case d.results <- internalResult{queueKey: key, task: task}:
	case <-ctx.Done():
	}
}

func (d *TaskDispatcher) waitForResult(ctx context.Context, executionID, taskID, sessionKey, workerID string) {
	deadline := time.Now().Add(pollTimeout)
	lastStatus := ""
	for time.Now().Before(deadline) {
		exec, err := d.execQuerier.GetByID(executionID)
		if err != nil {
			slog.Error("poll error", "component", "taskdispatcher", "execID", executionID, "error", err)
			return
		}
		if string(exec.Status) != lastStatus {
			slog.Info("polling execution", "component", "taskdispatcher", "execID", executionID, "status", exec.Status)
			lastStatus = string(exec.Status)
		}
		switch exec.Status {
		case model.ExecStatusCompleted:
			// Persist session_id for future resume (only on success).
			// Terminal task status is set by the worker via mark_task_success.
			if sessionKey != "" && workerID != "" {
				if err := d.sessionStore.UpsertSessionContext(ctx, sessionKey, workerID, exec.SessionID); err != nil {
					slog.Error("upsert session context", "component", "taskdispatcher", "error", err)
				}
			}
			return
		case model.ExecStatusFailed:
			// Terminal task status is set by the worker via mark_task_failed.
			return
		}
		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			return
		}
	}
}

func (d *TaskDispatcher) handleResult(res internalResult) {
	state, ok := d.queues[res.queueKey]
	if !ok {
		return
	}

	if len(state.pendingTasks) > 0 {
		next := state.pendingTasks[0]
		state.pendingTasks = state.pendingTasks[1:]
		nextReplyTo := next.ReplyTo
		state.lastReplyTo = nextReplyTo
		go d.executeAsync(d.ctx, res.queueKey, next, nextReplyTo)
	} else {
		state.executing = false
		delete(d.queues, res.queueKey)
	}
}
