// Package session contains domain operations on bee sessions that are reused
// by both IM slash-command handlers (/clear, /clear <worker>) and the RPC tool
// layer (clear_session, clear_worker_session). Keeping the destructive
// logic here ensures the two entry points stay in lock-step.
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

// FinalizeReasonCancelledByUser is the standard MarkAbandoned reason used by
// every stop-then-finalize fallback (clear_session, clear_worker, cancel_task).
const FinalizeReasonCancelledByUser = "cancelled by user"

// Op log labels identifying the caller of stop-then-finalize.
const (
	OpClearSession = "clear_session"
	OpClearWorker  = "clear_worker"
	OpCancelTask   = "cancel_task"
)

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

// ClearService performs the shared destructive operations behind /clear and
// ctl session clear*. Callers are responsible for their own confirmation UX —
// the service exposes a `force` parameter and returns running tasks so the
// caller can decide whether to gate execution.
type ClearService struct {
	sessions      SessionStore
	tasks         TaskStore
	execStopper   ExecutionStopper
	execFinalizer ExecutionFinalizer
	dispatcher    Dispatcher
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
		runningExecs:  deps.RunningExecs,
		engineCfg:     deps.EngineCfg,
	}
}

// ClearSessionResult describes the outcome (or pending state) of a session clear.
type ClearSessionResult struct {
	// Agents are the session contexts that would be (or were) cleared, scoped
	// to the currently active engine. Empty when the session has no contexts.
	Agents []store.SessionAgent
	// ActiveTasks holds the immediate pending+running tasks that share the
	// session. When Force=false and len(ActiveTasks)>0 the service stops here
	// without doing anything destructive, leaving the caller to gate.
	ActiveTasks []model.Task
	// CancelledTasks counts how many task rows transitioned to cancelled.
	CancelledTasks int64
	// Cleared is true when the destructive path actually ran.
	Cleared bool
}

// ClearSession evaluates and (when allowed) clears all session contexts for
// sessionKey scoped to the currently active engine. When force is false and
// active tasks exist, ClearSession returns them in ActiveTasks without
// touching anything, so the caller can show a confirmation. When the session
// has neither contexts nor tasks the call is a no-op.
func (s *ClearService) ClearSession(ctx context.Context, sessionKey string, force bool) (ClearSessionResult, error) {
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

	if len(agents) == 0 && len(activeTasks) == 0 {
		return ClearSessionResult{}, nil
	}

	if !force && len(activeTasks) > 0 {
		return ClearSessionResult{Agents: agents, ActiveTasks: activeTasks}, nil
	}

	s.stopRunningExecutions(ctx, activeTasks, OpClearSession)

	cancelled, err := s.tasks.Cancel(ctx, store.CancelFilter{
		SessionKey: sessionKey,
		Type:       model.TaskTypeImmediate,
	})
	if err != nil {
		return ClearSessionResult{Agents: agents, ActiveTasks: activeTasks},
			fmt.Errorf("cancel tasks for clear_session: %w", err)
	}

	s.dispatcher.ClearSession(sessionKey)

	return ClearSessionResult{
		Agents:         agents,
		CancelledTasks: cancelled,
		Cleared:        true,
	}, nil
}

// ClearWorkerResult describes the outcome (or pending state) of a worker-scoped clear.
type ClearWorkerResult struct {
	Worker         model.Worker
	Engine         string
	ActiveTasks    []model.Task
	CancelledTasks int64
	DeletedContext bool
	Cleared        bool
}

// ClearWorker evaluates and (when allowed) clears one worker's session context
// for the worker's active engine, cancels the worker's immediate tasks in the
// session, and drains the dispatcher's in-memory queue for the pair. As with
// ClearSession, an active-tasks check gates execution unless force is set.
func (s *ClearService) ClearWorker(ctx context.Context, sessionKey string, w model.Worker, force bool) (ClearWorkerResult, error) {
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

	if !force && len(activeTasks) > 0 {
		return ClearWorkerResult{Worker: w, Engine: engine, ActiveTasks: activeTasks}, nil
	}

	s.stopRunningExecutions(ctx, activeTasks, OpClearWorker)

	cancelled, err := s.tasks.Cancel(ctx, store.CancelFilter{
		SessionKey: sessionKey,
		WorkerID:   w.ID,
		Type:       model.TaskTypeImmediate,
	})
	if err != nil {
		return ClearWorkerResult{Worker: w, Engine: engine, ActiveTasks: activeTasks},
			fmt.Errorf("cancel tasks for clear_worker %s: %w", w.ID, err)
	}

	deleted, err := s.sessions.DeleteSessionContextForEngine(ctx, sessionKey, w.ID, engine)
	if err != nil {
		return ClearWorkerResult{}, err
	}

	s.dispatcher.ClearWorker(sessionKey, w.ID)

	return ClearWorkerResult{
		Worker:         w,
		Engine:         engine,
		CancelledTasks: cancelled,
		DeletedContext: deleted,
		Cleared:        true,
	}, nil
}

// stopRunningExecutions resolves the running exec ID for each task and stops
// it. When stop fails the row is force-finalized so future busy checks don't
// trip on a stale running row. Stops run concurrently so total clear latency
// scales with the slowest worker, not the sum.
func (s *ClearService) stopRunningExecutions(ctx context.Context, tasks []model.Task, op string) {
	execIDs := utils.RunningExecIDsForTasks(ctx, log, s.runningExecs, tasks, op)
	var wg sync.WaitGroup
	for _, t := range tasks {
		execID := execIDs[t.ID]
		if execID == "" {
			continue
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			s.StopAndFinalizeExecution(ctx, id, op)
		}(execID)
	}
	wg.Wait()
}

// StopAndFinalizeExecution stops a running execution; when the local stop
// reports an error (process gone, different instance), the row is
// force-finalized so future busy checks don't trip on a stale orphan. op is a
// log label identifying the caller (clear_session, clear_worker, cancel_task).
func (s *ClearService) StopAndFinalizeExecution(ctx context.Context, executionID, op string) {
	if executionID == "" {
		return
	}
	if err := s.execStopper.StopExecution(executionID); err != nil {
		log.Debug("stop execution: process not active",
			zap.String("op", op),
			zap.String("executionID", executionID),
			zap.Error(err))
		if s.execFinalizer != nil {
			if _, fErr := s.execFinalizer.MarkAbandoned(ctx, executionID, FinalizeReasonCancelledByUser); fErr != nil {
				log.Error("finalize cancelled execution",
					zap.String("op", op),
					zap.String("executionID", executionID), zap.Error(fErr))
			}
		}
	}
}
