// Package session contains domain operations on bee sessions reused by the
// IM slash-command handlers (/clear, /clear <worker>) and the RPC tool layer
// (clear_session, clear_worker_session, cancel_task).
package session

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

var log = logger.With(zap.String("component", "session"))

const finalizeReasonCancelledByUser = "cancelled by user"

type SessionStore interface {
	ListActiveSessionContexts(ctx context.Context, sessionKey, beeEngine string) ([]store.SessionAgent, error)
	DeleteSessionContextForEngine(ctx context.Context, sessionKey, agentID, engine string) (bool, error)
}

type TaskStore interface {
	List(ctx context.Context, f store.TaskFilter) ([]model.Task, error)
	Cancel(ctx context.Context, f store.CancelFilter) (int64, error)
}

// ExecutionStopper kills a running worker process by execution ID.
type ExecutionStopper interface {
	StopExecution(executionID string) error
}

// ExecutionFinalizer marks an execution row abandoned when the process is no
// longer alive in this instance.
type ExecutionFinalizer interface {
	MarkAbandoned(ctx context.Context, executionID, reason string) (bool, error)
}

// Dispatcher drains the dispatcher's in-memory queues for a session or worker.
// The destructive DB operations live in this service; Dispatcher methods are
// best-effort signals to the dispatcher's Run loop.
type Dispatcher interface {
	ClearSession(sessionKey string)
	ClearWorker(sessionKey, workerID string)
}

// TaskCanceller marks a single task cancelled in the DB and (for in-process
// implementations) drains it from the dispatcher's in-memory queue.
type TaskCanceller interface {
	CancelTask(ctx context.Context, taskID string) error
}

// ClearService performs the shared destructive operations behind /clear and
// ctl session clear*. Evaluate* methods are read-only previews; Clear* methods
// always execute. Callers gate the destructive path on their own UX.
type ClearService struct {
	sessions      SessionStore
	tasks         TaskStore
	execStopper   ExecutionStopper
	execFinalizer ExecutionFinalizer
	dispatcher    Dispatcher
	taskCanceller TaskCanceller
	runningExecs  utils.RunningExecLookup
	engineCfg     *enginecfg.Store
}

// ClearServiceDeps bundles the collaborators NewClearService needs.
type ClearServiceDeps struct {
	Sessions      SessionStore
	Tasks         TaskStore
	ExecStopper   ExecutionStopper
	ExecFinalizer ExecutionFinalizer
	Dispatcher    Dispatcher
	TaskCanceller TaskCanceller
	RunningExecs  utils.RunningExecLookup
	EngineCfg     *enginecfg.Store
}

func NewClearService(deps ClearServiceDeps) *ClearService {
	return &ClearService{
		sessions:      deps.Sessions,
		tasks:         deps.Tasks,
		execStopper:   deps.ExecStopper,
		execFinalizer: deps.ExecFinalizer,
		dispatcher:    deps.Dispatcher,
		taskCanceller: deps.TaskCanceller,
		runningExecs:  deps.RunningExecs,
		engineCfg:     deps.EngineCfg,
	}
}

// ClearSessionPreview is what EvaluateClearSession would touch. When both
// fields are empty the session has nothing to clear.
type ClearSessionPreview struct {
	Agents      []store.SessionAgent
	ActiveTasks []model.Task
}

// EvaluateClearSession is a read-only lookup of what a ClearSession call
// would clear. The destructive path is ClearSession.
func (s *ClearService) EvaluateClearSession(ctx context.Context, sessionKey string) (ClearSessionPreview, error) {
	beeEngine := s.engineCfg.Get()
	agents, err := s.sessions.ListActiveSessionContexts(ctx, sessionKey, beeEngine)
	if err != nil {
		return ClearSessionPreview{}, err
	}
	activeTasks, err := s.tasks.List(ctx, store.TaskFilter{
		SessionKey: sessionKey,
		Status:     model.TaskStatusActive,
		Type:       model.TaskTypeImmediate,
	})
	if err != nil {
		return ClearSessionPreview{}, err
	}
	return ClearSessionPreview{Agents: agents, ActiveTasks: activeTasks}, nil
}

// ClearSessionResult is the outcome of a ClearSession execution.
type ClearSessionResult struct {
	Agents         []store.SessionAgent
	CancelledTasks int64
}

// ClearSession stops the running executions, cancels immediate tasks, and
// drains the dispatcher queues for sessionKey. Always executes; callers are
// responsible for any confirmation UX.
func (s *ClearService) ClearSession(ctx context.Context, sessionKey string) (ClearSessionResult, error) {
	beeEngine := s.engineCfg.Get()
	agents, err := s.sessions.ListActiveSessionContexts(ctx, sessionKey, beeEngine)
	if err != nil {
		return ClearSessionResult{}, err
	}
	activeTasks, err := s.tasks.List(ctx, store.TaskFilter{
		SessionKey: sessionKey,
		Status:     model.TaskStatusActive,
		Type:       model.TaskTypeImmediate,
	})
	if err != nil {
		return ClearSessionResult{}, err
	}

	s.stopRunningExecutions(ctx, activeTasks)

	cancelled, err := s.tasks.Cancel(ctx, store.CancelFilter{
		SessionKey: sessionKey,
		Type:       model.TaskTypeImmediate,
	})
	if err != nil {
		return ClearSessionResult{Agents: agents}, fmt.Errorf("cancel tasks for clear_session: %w", err)
	}

	s.dispatcher.ClearSession(sessionKey)

	return ClearSessionResult{Agents: agents, CancelledTasks: cancelled}, nil
}

// ClearWorkerPreview is what EvaluateClearWorker would touch.
type ClearWorkerPreview struct {
	Engine      string
	ActiveTasks []model.Task
}

// EvaluateClearWorker is a read-only lookup of what a ClearWorker call would
// touch for one worker in sessionKey.
func (s *ClearService) EvaluateClearWorker(ctx context.Context, sessionKey string, w model.Worker) (ClearWorkerPreview, error) {
	engine := s.engineCfg.Resolve(w.Engine)
	activeTasks, err := s.tasks.List(ctx, store.TaskFilter{
		SessionKey: sessionKey,
		WorkerID:   w.ID,
		Status:     model.TaskStatusActive,
		Type:       model.TaskTypeImmediate,
	})
	if err != nil {
		return ClearWorkerPreview{}, err
	}
	return ClearWorkerPreview{Engine: engine, ActiveTasks: activeTasks}, nil
}

// ClearWorkerResult is the outcome of a ClearWorker execution.
type ClearWorkerResult struct {
	Engine         string
	CancelledTasks int64
	DeletedContext bool
}

// ClearWorker stops the running executions, cancels the worker's immediate
// tasks, deletes the session context for the worker's engine, and drains the
// dispatcher queue for the pair. Always executes.
func (s *ClearService) ClearWorker(ctx context.Context, sessionKey string, w model.Worker) (ClearWorkerResult, error) {
	engine := s.engineCfg.Resolve(w.Engine)

	activeTasks, err := s.tasks.List(ctx, store.TaskFilter{
		SessionKey: sessionKey,
		WorkerID:   w.ID,
		Status:     model.TaskStatusActive,
		Type:       model.TaskTypeImmediate,
	})
	if err != nil {
		return ClearWorkerResult{}, err
	}

	s.stopRunningExecutions(ctx, activeTasks)

	cancelled, err := s.tasks.Cancel(ctx, store.CancelFilter{
		SessionKey: sessionKey,
		WorkerID:   w.ID,
		Type:       model.TaskTypeImmediate,
	})
	if err != nil {
		return ClearWorkerResult{Engine: engine}, fmt.Errorf("cancel tasks for clear_worker %s: %w", w.ID, err)
	}

	deleted, err := s.sessions.DeleteSessionContextForEngine(ctx, sessionKey, w.ID, engine)
	if err != nil {
		return ClearWorkerResult{Engine: engine}, err
	}

	s.dispatcher.ClearWorker(sessionKey, w.ID)

	return ClearWorkerResult{
		Engine:         engine,
		CancelledTasks: cancelled,
		DeletedContext: deleted,
	}, nil
}

// CancelTask resolves the running execution for taskID, stops and finalizes
// it (if any), then marks the task cancelled via the TaskCanceller.
func (s *ClearService) CancelTask(ctx context.Context, taskID string) error {
	execIDs := utils.RunningExecIDsForTasks(ctx, log, s.runningExecs, []model.Task{{ID: taskID}})
	s.stopAndFinalizeExecution(ctx, execIDs[taskID])
	return s.taskCanceller.CancelTask(ctx, taskID)
}

// stopRunningExecutions resolves the running exec ID for each task and stops
// it. When stop fails the row is force-finalized so future busy checks don't
// trip on a stale running row. Stops run concurrently so total clear latency
// scales with the slowest worker, not the sum.
func (s *ClearService) stopRunningExecutions(ctx context.Context, tasks []model.Task) {
	execIDs := utils.RunningExecIDsForTasks(ctx, log, s.runningExecs, tasks)
	var wg sync.WaitGroup
	for _, t := range tasks {
		execID := execIDs[t.ID]
		if execID == "" {
			continue
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			s.stopAndFinalizeExecution(ctx, id)
		}(execID)
	}
	wg.Wait()
}

// stopAndFinalizeExecution stops a running execution; when the local stop
// reports an error (process gone, different instance), the row is
// force-finalized so future busy checks don't trip on a stale orphan.
func (s *ClearService) stopAndFinalizeExecution(ctx context.Context, executionID string) {
	if executionID == "" {
		return
	}
	if err := s.execStopper.StopExecution(executionID); err != nil {
		log.Debug("stop execution: process not active",
			zap.String("executionID", executionID),
			zap.Error(err))
		if s.execFinalizer != nil {
			if _, fErr := s.execFinalizer.MarkAbandoned(ctx, executionID, finalizeReasonCancelledByUser); fErr != nil {
				log.Error("finalize cancelled execution",
					zap.String("executionID", executionID), zap.Error(fErr))
			}
		}
	}
}
