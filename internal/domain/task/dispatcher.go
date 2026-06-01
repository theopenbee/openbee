package task

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

var log = logger.With(zap.String("component", "taskdispatcher"))

type taskMeta struct {
	MessageID       string          `json:"message_id"`
	TaskID          string          `json:"task_id,omitempty"`
	PlatformContext json.RawMessage `json:"platform_context,omitempty"`
}

const (
	pollInterval = 2 * time.Second
)

// ExecutionManager manages worker executions.
type ExecutionManager interface {
	ExecuteWorker(ctx context.Context, workerID, taskID, input, sessionID string, resume bool) (model.WorkerExecution, error)
	CancelExecution(ctx context.Context, executionID string) error
}

// ExecutionQuerier retrieves execution state by ID.
type ExecutionQuerier interface {
	GetByID(id string) (model.WorkerExecution, error)
}

// TaskStore is the subset of store.TaskStore used by the TaskDispatcher.
type TaskStore interface {
	UpdateStatus(ctx context.Context, taskID, status string) error
	CompleteTask(ctx context.Context, taskID string) error
	FailTask(ctx context.Context, taskID string) error
	CancelTask(ctx context.Context, taskID string) error
}

// FailureNotifier sends failure and cancellation notifications to users.
type FailureNotifier interface {
	NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error
	NotifyTaskCancelled(ctx context.Context, messageID string, workerName string) error
}

// SessionStore is the subset of store.SessionStore used by the TaskDispatcher.
type SessionStore interface {
	GetSessionContextForEngine(ctx context.Context, sessionKey, agentID, engine string) (sessionID string, err error)
	UpsertSessionContext(ctx context.Context, sessionKey, agentID, sessionID, engine string) error
	DeleteSessionContextForEngine(ctx context.Context, sessionKey, agentID, engine string) (bool, error)
	ClearSessionContexts(ctx context.Context, sessionKey, beeEngine string) error
}

// WorkerLookup fetches worker metadata for persona injection on new sessions.
type WorkerLookup interface {
	GetByID(id string) (model.Worker, error)
}

type queueState struct {
	executing    bool
	pendingTasks []DispatchTask
}

type internalResult struct {
	queueKey string
	task     DispatchTask
}

type clearWorkerRequest struct {
	sessionKey string
	workerID   string
}

// TaskDispatcher serializes worker executions per WorkerID.
type TaskDispatcher struct {
	ctx             context.Context               // injected by Run; controls the dispatcher lifecycle
	manager         ExecutionManager              // launches worker executions
	taskStore       TaskStore                     // persists task-to-execution mapping and state
	sessionStore    SessionStore                  // reads, writes, and cleans up session contexts
	execStore       ExecutionQuerier              // queries execution state by ID
	engineCfg       *enginecfg.Store              // resolves the current default engine
	failureNotifier FailureNotifier               // sends failure notifications (optional)
	workerLookup    WorkerLookup                  // optional; if nil, no persona is injected
	inCh            <-chan DispatchTask           // inbound task channel
	resultsCh       chan internalResult           // internal completion signal channel; drives queue scheduling
	queues          map[string]*queueState        // per-workerID serial queues
	clearCh         chan string                   // receives sessionKey signals that need to be cleaned up
	clearWorkerCh   chan clearWorkerRequest       // receives (sessionKey, workerID) signals to clear a single worker's queue
	cancelFuncs     map[string]context.CancelFunc // taskID → cancel func; owned by Run loop
	cancelCh        chan string                   // receives taskID cancel requests
}

// New constructs a TaskDispatcher.
func New(manager ExecutionManager, taskStore TaskStore, sessionStore SessionStore, execStore ExecutionQuerier, in <-chan DispatchTask, engineCfg *enginecfg.Store, opts ...Option) *TaskDispatcher {
	d := &TaskDispatcher{
		manager:      manager,
		taskStore:    taskStore,
		sessionStore: sessionStore,
		execStore:    execStore,
		engineCfg:    engineCfg,
		inCh:         in,
		resultsCh:    make(chan internalResult, 64),
		queues:       make(map[string]*queueState),
		clearCh:       make(chan string, 8),
		clearWorkerCh: make(chan clearWorkerRequest, 16),
		cancelFuncs:   make(map[string]context.CancelFunc),
		cancelCh:      make(chan string, 16),
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

// WithWorkerLookup sets the lookup used to fetch worker metadata for persona injection.
func WithWorkerLookup(lookup WorkerLookup) Option {
	return func(d *TaskDispatcher) { d.workerLookup = lookup }
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
		case req := <-d.clearWorkerCh:
			d.clearWorkerQueue(req.sessionKey, req.workerID)
		case taskID := <-d.cancelCh:
			d.handleCancel(taskID)
		case <-ctx.Done():
			return
		}
	}
}

func (d *TaskDispatcher) handleInbound(task DispatchTask) {
	key := task.WorkerID
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

// ClearWorker removes queued tasks for the given (sessionKey, workerID) pair from the
// dispatcher's in-memory queues. Safe to call from any goroutine.
//
// The caller is responsible for stopping any currently executing worker via
// StopExecution and marking tasks cancelled in the database; this method only
// drains the pending tail of the queue so no future tasks fire for that pair.
func (d *TaskDispatcher) ClearWorker(sessionKey, workerID string) {
	select {
	case d.clearWorkerCh <- clearWorkerRequest{sessionKey: sessionKey, workerID: workerID}:
	default:
		log.Warn("clearWorkerCh full, dropping clear",
			zap.String("sessionKey", sessionKey),
			zap.String("workerID", workerID))
	}
}

// ClearSession removes all queued tasks for the given session and clears session contexts.
// Safe to call from any goroutine — uses a buffered channel to signal the Run loop.
func (d *TaskDispatcher) ClearSession(sessionKey string) {
	// Clear DB synchronously so feeder can detect the clear after bee exits.
	if err := d.sessionStore.ClearSessionContexts(context.Background(), sessionKey, d.engineCfg.Get()); err != nil {
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

// dropQueued removes pending tasks for which keep(task) is false from every queue.
// Empty, idle queues are deleted from d.queues.
func (d *TaskDispatcher) dropQueued(keep func(DispatchTask) bool) {
	for key, state := range d.queues {
		var remaining []DispatchTask
		for _, t := range state.pendingTasks {
			if keep(t) {
				remaining = append(remaining, t)
			}
		}
		state.pendingTasks = remaining
		if !state.executing && len(state.pendingTasks) == 0 {
			delete(d.queues, key)
		}
	}
}

// clearWorkerQueue drops queued (not-yet-executing) tasks for the (sessionKey, workerID)
// pair. Tasks already running keep going — the command layer stops their executions
// and cancels them in the database separately.
func (d *TaskDispatcher) clearWorkerQueue(sessionKey, workerID string) {
	d.dropQueued(func(t DispatchTask) bool {
		return t.SessionKey != sessionKey || t.WorkerID != workerID
	})
}

func (d *TaskDispatcher) clearQueues(sessionKey string) {
	d.dropQueued(func(t DispatchTask) bool { return t.SessionKey != sessionKey })
}

// buildInstruction prepends task metadata to the instruction so workers
// can call mark_task_success and send_message via RPC.
func buildInstruction(t DispatchTask) string {
	if t.TaskID != "" || t.MessageID != "" {
		meta := taskMeta{
			MessageID: t.MessageID,
			TaskID:    t.TaskID,
		}
		if ctx := platform.ExtractContext(t.ReplyTo.Platform, t.ReplyTo.Raw); ctx != "" {
			meta.PlatformContext = json.RawMessage(ctx)
		}
		b, _ := json.Marshal(meta)
		return fmt.Sprintf("<task_meta>%s</task_meta>\n<task_content>\n%s\n</task_content>", b, t.Instruction)
	}
	return t.Instruction
}

func (d *TaskDispatcher) executeAsync(taskCtx context.Context, cancel context.CancelFunc, key string, task DispatchTask) {
	defer cancel() // always release the cancel func's resources
	defer func() {
		select {
		case d.resultsCh <- internalResult{queueKey: key, task: task}:
		case <-d.ctx.Done():
		}
	}()

	engineName, worker := d.resolveWorkerEngine(task.WorkerID)
	instruction := buildInstruction(task)
	exec, err := d.resolveExecution(taskCtx, task, instruction, engineName, worker)
	if err != nil {
		log.Error("execute error",
			zap.String("workerID", task.WorkerID),
			zap.String("taskID", task.TaskID),
			zap.String("messageID", task.MessageID),
			zap.String("taskType", task.TaskType),
			zap.String("sessionKey", task.SessionKey),
			zap.Error(err),
		)
		if task.TaskID != "" {
			if failErr := d.taskStore.FailTask(taskCtx, task.TaskID); failErr != nil {
				log.Error("fail task after execute error", zap.String("taskID", task.TaskID), zap.Error(failErr))
			}
		}
		d.notifyFailure(taskCtx, task.MessageID, model.FailureInfo{
			Reason:     err.Error(),
			WorkerName: task.WorkerID,
		})
		return
	}

	// Cancellation may have arrived while resolveExecution was in-flight; kill the
	// just-launched worker before entering waitForResult.
	if taskCtx.Err() != nil {
		d.manager.CancelExecution(context.Background(), exec.ID) //nolint:errcheck
		d.notifyCancel(context.Background(), task.MessageID, workerName(exec.WorkerName, task.WorkerID))
		return
	}

	if task.TaskID != "" {
		if err := d.taskStore.UpdateStatus(taskCtx, task.TaskID, model.TaskStatusRunning); err != nil {
			log.Error("update task status", zap.String("taskID", task.TaskID), zap.Error(err))
		}
	}
	d.waitForResult(taskCtx, exec.ID, task, engineName)
}

// resolveWorkerEngine returns the engine name and the fetched worker (nil if
// workerLookup is unavailable or the lookup fails). A single DB call covers
// both engine selection and the persona injection needed by executeFresh.
func (d *TaskDispatcher) resolveWorkerEngine(workerID string) (string, *model.Worker) {
	if d.workerLookup != nil {
		w, err := d.workerLookup.GetByID(workerID)
		if err != nil {
			log.Warn("worker lookup failed, falling back to default engine",
				zap.String("workerID", workerID), zap.Error(err))
			return d.engineCfg.Get(), nil
		}
		return d.engineCfg.Resolve(w.Engine), &w
	}
	return d.engineCfg.Get(), nil
}

// executeFresh builds the session prefix (Step 1 + persona) and starts a fresh
// execution. worker is the pre-fetched record from resolveWorkerEngine; if
// workerLookup is configured but worker is nil, the lookup failed and the task
// is aborted.
func (d *TaskDispatcher) executeFresh(ctx context.Context, task DispatchTask, instruction, engineName string, worker *model.Worker) (model.WorkerExecution, error) {
	persona := ""
	if d.workerLookup != nil {
		if worker == nil {
			return model.WorkerExecution{}, fmt.Errorf("worker %q not found", task.WorkerID)
		}
		persona = ai.WorkerPersona(worker.Name, worker.Description, worker.Constraints)
	}
	prefix := ai.BuildWorkerSessionPrefix(persona)
	sessionID := uuid.New().String()
	d.upsertSessionContext(ctx, task, sessionID, engineName)
	log.Info("executing worker", zap.String("workerID", task.WorkerID), zap.String("taskID", task.TaskID))
	return d.manager.ExecuteWorker(ctx, task.WorkerID, task.TaskID, prefix+instruction, sessionID, false)
}

func (d *TaskDispatcher) resolveExecution(ctx context.Context, task DispatchTask, instruction, engineName string, worker *model.Worker) (model.WorkerExecution, error) {
	if task.TaskType != model.TaskTypeImmediate {
		return d.executeFresh(ctx, task, instruction, engineName, worker)
	}
	sessionID, err := d.sessionStore.GetSessionContextForEngine(ctx, task.SessionKey, task.WorkerID, engineName)
	if err != nil {
		log.Error("get session context", zap.Error(err))
	}
	if sessionID == "" {
		return d.executeFresh(ctx, task, instruction, engineName, worker)
	}
	log.Info("resuming session", zap.String("sessionID", sessionID), zap.String("taskID", task.TaskID))
	d.upsertSessionContext(ctx, task, sessionID, engineName)
	exec, err := d.manager.ExecuteWorker(ctx, task.WorkerID, task.TaskID, instruction, sessionID, true)
	if err == nil {
		return exec, nil
	}
	log.Error("resume error, falling back to fresh", zap.Error(err))
	if task.SessionKey != "" && task.WorkerID != "" {
		if _, clearErr := d.sessionStore.DeleteSessionContextForEngine(ctx, task.SessionKey, task.WorkerID, engineName); clearErr != nil {
			log.Error("clear stale session context", zap.String("sessionKey", task.SessionKey), zap.String("workerID", task.WorkerID), zap.String("engine", engineName), zap.Error(clearErr))
		}
	}
	return d.executeFresh(ctx, task, instruction, engineName, worker)
}

// waitForResult polls the execution until it reaches a terminal state or ctx
// is cancelled. Process-level timeouts are enforced by launchRuntime via
// workerTimeout; this loop must not impose its own deadline, or it would exit
// while the worker keeps running and leave the task row stuck in `running`.
func (d *TaskDispatcher) waitForResult(ctx context.Context, executionID string, task DispatchTask, engineName string) {
	lastStatus := ""
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		exec, err := d.execStore.GetByID(executionID)
		if err != nil {
			log.Error("poll error", zap.String("execID", executionID), zap.Error(err))
			if ctx.Err() != nil {
				d.manager.CancelExecution(context.Background(), executionID) //nolint:errcheck
			}
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
			d.upsertSessionContext(ctx, task, exec.SessionID, engineName)
			return
		case model.ExecStatusFailed:
			// Persist session context even on failure so the next dispatch can attempt
			// to resume. If resume also fails, resolveExecution will clear and retry fresh.
			d.upsertSessionContext(ctx, task, exec.SessionID, engineName)
			// Dispatcher sets terminal task status on abnormal worker exit.
			if task.TaskID != "" {
				if err := d.taskStore.FailTask(ctx, task.TaskID); err != nil {
					log.Error("fail task", zap.String("taskID", task.TaskID), zap.Error(err))
				}
			}
			d.notifyFailure(ctx, task.MessageID, model.FailureInfo{
				Reason:     exec.Result,
				WorkerName: workerName(exec.WorkerName, task.WorkerID),
			})
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			d.manager.CancelExecution(context.Background(), executionID) //nolint:errcheck
			d.notifyCancel(context.Background(), task.MessageID, workerName(exec.WorkerName, task.WorkerID))
			return
		}
	}
}

func (d *TaskDispatcher) upsertSessionContext(ctx context.Context, task DispatchTask, sessionID, engineName string) {
	if task.SessionKey == "" || task.WorkerID == "" || sessionID == "" {
		return
	}
	if err := d.sessionStore.UpsertSessionContext(ctx, task.SessionKey, task.WorkerID, sessionID, engineName); err != nil {
		log.Error("upsert session context", zap.String("sessionKey", task.SessionKey), zap.Error(err))
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

func (d *TaskDispatcher) notifyCancel(ctx context.Context, messageID, workerName string) {
	if d.failureNotifier == nil || messageID == "" {
		return
	}
	if err := d.failureNotifier.NotifyTaskCancelled(ctx, messageID, workerName); err != nil {
		log.Error("notify task cancel", zap.String("messageID", messageID), zap.Error(err))
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
