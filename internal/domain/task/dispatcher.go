package task

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
)

var log = logger.With(zap.String("component", "taskdispatcher"))

const (
	pollInterval = 2 * time.Second
	pollTimeout  = 30 * time.Minute
)

// ExecutionManager manages worker executions.
type ExecutionManager interface {
	ExecuteWorker(ctx context.Context, workerID, input, sessionID string) (model.WorkerExecution, error)
	CancelExecution(ctx context.Context, executionID string) error
}

// ExecutionQuerier retrieves execution state by ID.
type ExecutionQuerier interface {
	GetByID(id string) (model.WorkerExecution, error)
}

// TaskStore is the subset of store.TaskStore used by the TaskDispatcher.
type TaskStore interface {
	SetExecution(ctx context.Context, taskID, executionID, status string) error
	CompleteTask(ctx context.Context, taskID string) error
	FailTask(ctx context.Context, taskID string) error
	CancelTask(ctx context.Context, taskID string) error
}

// FailureNotifier sends failure notifications to users when a worker execution
// fails at the system level (e.g. API error, content filtering) and the worker
// itself had no chance to call send_message.
type FailureNotifier interface {
	NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error
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
	ctx              context.Context                  // injected by Run; controls the dispatcher lifecycle
	manager          ExecutionManager                 // launches worker executions
	taskStore        TaskStore                        // persists task-to-execution mapping and state
	sessionStore     SessionStore                     // reads, writes, and cleans up session contexts
	execStore        ExecutionQuerier                 // queries execution state by ID
	failureNotifier  FailureNotifier                  // sends failure notifications (optional)
	inCh             <-chan DispatchTask              // inbound task channel
	resultsCh        chan internalResult              // internal completion signal channel; drives queue scheduling
	queues           map[string]*queueState           // per-workerID serial queues
	clearCh          chan string                      // receives sessionKey signals that need to be cleaned up
	cancelFuncs      map[string]context.CancelFunc   // taskID → cancel func; owned by Run loop
	cancelCh         chan string                      // receives taskID cancel requests
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
		cancelFuncs:  make(map[string]context.CancelFunc),
		cancelCh:     make(chan string, 16),
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
		case taskID := <-d.cancelCh:
			d.handleCancel(taskID)
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
		d.startTask(key, task)
	} else {
		state.pendingTasks = append(state.pendingTasks, task)
	}
}

func (d *TaskDispatcher) startTask(key string, task DispatchTask) {
	taskCtx, cancel := context.WithCancel(d.ctx)
	if task.TaskID != "" {
		d.cancelFuncs[task.TaskID] = cancel
	}
	go d.executeAsync(taskCtx, cancel, key, task)
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

func (d *TaskDispatcher) handleCancel(taskID string) {
	// Remove from any pending queue
	for key, state := range d.queues {
		var remaining []DispatchTask
		for _, t := range state.pendingTasks {
			if t.TaskID != taskID {
				remaining = append(remaining, t)
			}
		}
		state.pendingTasks = remaining
		if !state.executing && len(state.pendingTasks) == 0 {
			delete(d.queues, key)
		}
	}
	// Interrupt executing goroutine if present
	if cancel, ok := d.cancelFuncs[taskID]; ok {
		cancel()
		delete(d.cancelFuncs, taskID)
	}
}

// CancelTask marks the task cancelled in DB and signals the Run loop to
// remove it from the in-memory queue or interrupt its executing goroutine.
// Best-effort: returns once DB is updated; goroutine interruption is async.
func (d *TaskDispatcher) CancelTask(ctx context.Context, taskID string) error {
	if err := d.taskStore.CancelTask(ctx, taskID); err != nil {
		return err
	}
	select {
	case d.cancelCh <- taskID:
	default:
		log.Warn("cancelCh full, in-memory cancel dropped", zap.String("taskID", taskID))
	}
	return nil
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
func buildInstruction(t DispatchTask) string {
	if t.TaskID == "" && t.MessageID == "" {
		return t.Instruction
	}
	if t.TaskID == "" {
		return fmt.Sprintf("---\nmessage_id: %s\n---\n\n%s", t.MessageID, t.Instruction)
	}
	return fmt.Sprintf("---\nmessage_id: %s\ntask_id: %s\n---\n\n%s", t.MessageID, t.TaskID, t.Instruction)
}

func (d *TaskDispatcher) executeAsync(taskCtx context.Context, cancel context.CancelFunc, key string, task DispatchTask) {
	defer cancel() // always release the cancel func's resources
	defer func() {
		select {
		case d.resultsCh <- internalResult{queueKey: key, task: task}:
		case <-d.ctx.Done():
		}
	}()

	instruction := buildInstruction(task)
	exec, err := d.resolveExecution(taskCtx, task, instruction)
	if err != nil {
		log.Error("execute error", zap.Error(err))
		if task.TaskID != "" {
			if failErr := d.taskStore.FailTask(taskCtx, task.TaskID); failErr != nil {
				log.Error("fail task after execute error", zap.String("taskID", task.TaskID), zap.Error(failErr))
			}
		}
		d.notifyFailure(taskCtx, task.MessageID, model.FailureInfo{
			Reason:     err.Error(),
			WorkerName: task.WorkerID,
			RetryCount: -1,
		})
		return
	}

	// Cancellation may have arrived while resolveExecution was in-flight; kill the
	// just-launched worker before entering waitForResult.
	if taskCtx.Err() != nil {
		d.manager.CancelExecution(context.Background(), exec.ID) //nolint:errcheck
		return
	}

	if task.TaskID != "" {
		if err := d.taskStore.SetExecution(taskCtx, task.TaskID, exec.ID, model.TaskStatusRunning); err != nil {
			log.Error("set execution", zap.String("taskID", task.TaskID), zap.Error(err))
		}
	}
	d.waitForResult(taskCtx, exec.ID, task)
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
			if task.TaskID != "" {
				if err := d.taskStore.CompleteTask(ctx, task.TaskID); err != nil {
					log.Error("complete task", zap.String("taskID", task.TaskID), zap.Error(err))
				}
			}
			// Persist session_id for future resume (only on success).
			if task.SessionKey != "" && task.WorkerID != "" {
				if err := d.sessionStore.UpsertSessionContext(ctx, task.SessionKey, task.WorkerID, exec.SessionID); err != nil {
					log.Error("upsert session context", zap.Error(err))
				}
			}
			return
		case model.ExecStatusFailed:
			// Persist session context even on failure so the next dispatch can attempt
			// to resume. If resume also fails, resolveExecution will clear and retry fresh.
			if task.SessionKey != "" && task.WorkerID != "" {
				if err := d.sessionStore.UpsertSessionContext(ctx, task.SessionKey, task.WorkerID, exec.SessionID); err != nil {
					log.Error("upsert session context on failure", zap.Error(err))
				}
			}
			// Dispatcher sets terminal task status on abnormal worker exit.
			if task.TaskID != "" {
				if err := d.taskStore.FailTask(ctx, task.TaskID); err != nil {
					log.Error("fail task", zap.String("taskID", task.TaskID), zap.Error(err))
				}
			}
			d.notifyFailure(ctx, task.MessageID, model.FailureInfo{
				Reason:     exec.Result,
				WorkerName: workerName(exec.WorkerName, task.WorkerID),
				RetryCount: -1,
			})
			return
		}
		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			// Task was cancelled — kill the worker process.
			d.manager.CancelExecution(context.Background(), executionID) //nolint:errcheck
			return
		}
	}
}

func workerName(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

func (d *TaskDispatcher) notifyFailure(ctx context.Context, messageID string, info model.FailureInfo) {
	if d.failureNotifier == nil || messageID == "" {
		return
	}
	if err := d.failureNotifier.NotifyTaskFailure(ctx, messageID, info); err != nil {
		log.Error("notify task failure", zap.String("messageID", messageID), zap.Error(err))
	}
}

func (d *TaskDispatcher) handleResult(res internalResult) {
	delete(d.cancelFuncs, res.task.TaskID)

	state, ok := d.queues[res.queueKey]
	if !ok {
		return
	}

	if len(state.pendingTasks) > 0 {
		next := state.pendingTasks[0]
		state.pendingTasks = state.pendingTasks[1:]
		d.startTask(res.queueKey, next)
	} else {
		state.executing = false
		delete(d.queues, res.queueKey)
	}
}
