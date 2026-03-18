package task_dispatcher

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/logger"
	"github.com/theopenbee/openbee/internal/model"
)

var log = logger.With(zap.String("component", "taskdispatcher"))

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
	FailTask(ctx context.Context, taskID string) error
}

// FailureNotifier sends failure notifications to users when a worker execution
// fails at the system level (e.g. API error, content filtering) and the worker
// itself had no chance to call send_message.
type FailureNotifier interface {
	NotifyTaskFailure(ctx context.Context, messageID, reason string) error
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
}

type internalResult struct {
	queueKey string
	task     DispatchTask
}

// TaskDispatcher serializes worker executions per WorkerID.
type TaskDispatcher struct {
	ctx              context.Context        // 由 Run 注入的生命周期上下文
	manager          ExecutionManager       // 启动 worker 执行
	taskStore        TaskStore              // 持久化 task 与 execution 的关联状态
	sessionStore     SessionStore           // 管理会话上下文的读写与清理
	execStore        ExecutionQuerier       // 按 ID 查询 execution 状态
	failureNotifier  FailureNotifier        // 发送失败通知（可选）
	inCh             <-chan DispatchTask    // 入站任务通道
	resultsCh        chan internalResult    // 内部完成信号通道，用于驱动队列调度
	queues           map[string]*queueState // 按 workerID 分组的串行队列
	clearCh          chan string            // 接收需要清理的 sessionKey 信号
}

// New constructs a TaskDispatcher.
func New(manager ExecutionManager, taskStore TaskStore, sessionStore SessionStore, execStore ExecutionQuerier, in <-chan DispatchTask, opts ...Option) *TaskDispatcher {
	d := &TaskDispatcher{
		manager:      manager,
		taskStore:    taskStore,
		sessionStore: sessionStore,
		execStore:    execStore,
		inCh:         in,
		resultsCh:    make(chan internalResult, 64),
		queues:       make(map[string]*queueState),
		clearCh:      make(chan string, 8),
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Option configures a TaskDispatcher.
type Option func(*TaskDispatcher)

// WithFailureNotifier sets the notifier used to inform users about task failures.
func WithFailureNotifier(fn FailureNotifier) Option {
	return func(d *TaskDispatcher) { d.failureNotifier = fn }
}

// Run processes tasks until ctx is cancelled. Call in a goroutine.
func (d *TaskDispatcher) Run(ctx context.Context) {
	d.ctx = ctx
	for {
		select {
		case task, ok := <-d.inCh:
			if !ok {
				return
			}
			d.handleInbound(task)
		case res := <-d.resultsCh:
			d.handleResult(res)
		case sessionKey := <-d.clearCh:
			d.clearQueues(sessionKey)
		case <-ctx.Done():
			return
		}
	}
}

func queueKey(_, workerID string) string {
	return workerID
}

// ExportedQueueKey is exported for testing only.
var ExportedQueueKey = queueKey

func (d *TaskDispatcher) handleInbound(task DispatchTask) {
	key := queueKey(task.SessionKey, task.WorkerID)
	state, ok := d.queues[key]
	if !ok {
		state = &queueState{}
		d.queues[key] = state
	}

	if !state.executing {
		state.executing = true
		go d.executeAsync(d.ctx, key, task)
	} else {
		state.pendingTasks = append(state.pendingTasks, task)
	}
}

// ClearSession removes all queued tasks for the given session and clears session contexts.
// Safe to call from any goroutine — uses a buffered channel to signal the Run loop.
func (d *TaskDispatcher) ClearSession(sessionKey string) {
	// Clear DB synchronously so feeder can detect the clear after bee exits.
	if err := d.sessionStore.ClearSessionContexts(context.Background(), sessionKey); err != nil {
		log.Error("clear session contexts", zap.String("sessionKey", sessionKey), zap.Error(err))
	}
	// Signal Run loop to clear in-memory queues.
	select {
	case d.clearCh <- sessionKey:
	default:
		log.Warn("clearCh full, dropping clear", zap.String("sessionKey", sessionKey))
	}
}

func (d *TaskDispatcher) clearQueues(sessionKey string) {
	for key, state := range d.queues {
		var remaining []DispatchTask
		for _, t := range state.pendingTasks {
			if t.SessionKey != sessionKey {
				remaining = append(remaining, t)
			}
		}
		state.pendingTasks = remaining
		if !state.executing && len(state.pendingTasks) == 0 {
			delete(d.queues, key)
		}
	}
	if err := d.sessionStore.ClearSessionContexts(d.ctx, sessionKey); err != nil {
		log.Error("clear session contexts", zap.String("sessionKey", sessionKey), zap.Error(err))
	}
}

// buildInstruction prepends task metadata to the instruction so workers
// can call mark_task_success and send_message via MCP.
func buildInstruction(task DispatchTask) string {
	if task.TaskID == "" {
		return task.Instruction
	}
	return fmt.Sprintf("[系统元数据] task_id=%s message_id=%s\n\n%s",
		task.TaskID, task.MessageID, task.Instruction)
}

func (d *TaskDispatcher) executeAsync(ctx context.Context, key string, task DispatchTask) {
	instruction := buildInstruction(task)
	defer func() {
		select {
		case d.resultsCh <- internalResult{queueKey: key, task: task}:
		case <-ctx.Done():
		}
	}()

	exec, err := d.resolveExecution(ctx, task, instruction)
	if err != nil {
		log.Error("execute error", zap.Error(err))
		if task.TaskID != "" {
			if failErr := d.taskStore.FailTask(ctx, task.TaskID); failErr != nil {
				log.Error("fail task after execute error", zap.String("taskID", task.TaskID), zap.Error(failErr))
			}
		}
		d.notifyFailure(ctx, task.MessageID, err.Error())
		return
	}
	if task.TaskID != "" {
		if err := d.taskStore.SetExecution(ctx, task.TaskID, exec.ID, model.TaskStatusRunning); err != nil {
			log.Error("set execution", zap.String("taskID", task.TaskID), zap.Error(err))
		}
	}
	d.waitForResult(ctx, exec.ID, task)
}

func (d *TaskDispatcher) resolveExecution(ctx context.Context, task DispatchTask, instruction string) (model.WorkerExecution, error) {
	if task.TaskType != model.TaskTypeImmediate {
		log.Info("executing worker", zap.String("workerID", task.WorkerID), zap.String("taskID", task.TaskID))
		return d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, "")
	}
	sessionID, err := d.sessionStore.GetSessionContext(ctx, task.SessionKey, task.WorkerID)
	if err != nil {
		log.Error("get session context", zap.Error(err))
	}
	if sessionID == "" {
		log.Info("executing worker", zap.String("workerID", task.WorkerID), zap.String("taskID", task.TaskID))
		return d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, "")
	}
	log.Info("resuming session", zap.String("sessionID", sessionID), zap.String("taskID", task.TaskID))
	exec, err := d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, sessionID)
	if err == nil {
		return exec, nil
	}
	log.Error("resume error, falling back to fresh", zap.Error(err))
	if clearErr := d.sessionStore.ClearSessionContexts(ctx, task.SessionKey); clearErr != nil {
		log.Error("clear stale session contexts", zap.String("sessionKey", task.SessionKey), zap.Error(clearErr))
	}
	return d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, "")
}

func (d *TaskDispatcher) waitForResult(ctx context.Context, executionID string, task DispatchTask) {
	deadline := time.Now().Add(pollTimeout)
	lastStatus := ""
	for time.Now().Before(deadline) {
		exec, err := d.execStore.GetByID(executionID)
		if err != nil {
			log.Error("poll error", zap.String("execID", executionID), zap.Error(err))
			return
		}
		if string(exec.Status) != lastStatus {
			log.Info("polling execution", zap.String("execID", executionID), zap.Any("status", exec.Status))
			lastStatus = string(exec.Status)
		}
		switch exec.Status {
		case model.ExecStatusCompleted:
			// Persist session_id for future resume (only on success).
			// Terminal task status is set by the worker via mark_task_success.
			if task.SessionKey != "" && task.WorkerID != "" {
				if err := d.sessionStore.UpsertSessionContext(ctx, task.SessionKey, task.WorkerID, exec.SessionID); err != nil {
					log.Error("upsert session context", zap.Error(err))
				}
			}
			return
		case model.ExecStatusFailed:
			// Dispatcher sets terminal task status on abnormal worker exit.
			if task.TaskID != "" {
				if err := d.taskStore.FailTask(ctx, task.TaskID); err != nil {
					log.Error("fail task", zap.String("taskID", task.TaskID), zap.Error(err))
				}
			}
			d.notifyFailure(ctx, task.MessageID, exec.Result)
			return
		}
		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			return
		}
	}
}

func (d *TaskDispatcher) notifyFailure(ctx context.Context, messageID, reason string) {
	if d.failureNotifier == nil || messageID == "" {
		return
	}
	if err := d.failureNotifier.NotifyTaskFailure(ctx, messageID, reason); err != nil {
		log.Error("notify task failure", zap.String("messageID", messageID), zap.Error(err))
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
		go d.executeAsync(d.ctx, res.queueKey, next)
	} else {
		state.executing = false
		delete(d.queues, res.queueKey)
	}
}
