package task

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

// reconcilerTaskStore is the subset of store.TaskStore used by the Reconciler.
type reconcilerTaskStore interface {
	List(ctx context.Context, f store.TaskFilter) ([]model.Task, error)
	CompleteTask(ctx context.Context, taskID string) error
	FailTask(ctx context.Context, taskID string) error
}

// reconcilerExecStore is the subset of store.ExecutionStore used by the Reconciler.
type reconcilerExecStore interface {
	GetRunningByTaskID(ctx context.Context, taskID string) (*model.WorkerExecution, error)
	ListByTaskIDs(ctx context.Context, taskIDs []string, limitPerTask int) (map[string][]model.WorkerExecution, error)
	MarkAbandoned(ctx context.Context, id, result string) (bool, error)
}

// Reconciler runs in the background and repairs tasks whose status drifted
// out of sync with their underlying execution row. Drift can happen when:
//   - dispatcher's waitForResult exits before the exec reaches a terminal state
//     (no longer the default behavior, but a safety net for in-flight tasks),
//   - monitorExecution misses the output channel close (engine adapter bug),
//   - the worker process is killed by the OS without the runtime noticing.
//
// Without this loop, /status keeps reporting stale "Running tasks" until the
// next server restart, when ResetRunningExecutions sweeps the DB.
type Reconciler struct {
	taskStore reconcilerTaskStore
	execStore reconcilerExecStore
	interval  time.Duration
	// processAlive overrides the default OS process liveness check in tests.
	processAlive func(pid int) bool
}

// NewReconciler constructs a Reconciler. interval <= 0 falls back to 60s.
func NewReconciler(taskStore reconcilerTaskStore, execStore reconcilerExecStore, interval time.Duration) *Reconciler {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Reconciler{
		taskStore:    taskStore,
		execStore:    execStore,
		interval:     interval,
		processAlive: utils.IsProcessAlive,
	}
}

// Run reconciles on each tick until ctx is cancelled. Call in a goroutine.
func (r *Reconciler) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.reconcile(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (r *Reconciler) reconcile(ctx context.Context) {
	tasks, err := r.taskStore.List(ctx, store.TaskFilter{Status: model.TaskStatusRunning})
	if err != nil {
		log.Error("reconciler: list running tasks", zap.Error(err))
		return
	}
	for _, t := range tasks {
		r.reconcileOne(ctx, t)
	}
}

func (r *Reconciler) reconcileOne(ctx context.Context, t model.Task) {
	latest, err := r.latestExecution(ctx, t.ID)
	if err != nil {
		log.Error("reconciler: get execution",
			zap.String("taskID", t.ID), zap.Error(err))
		return
	}
	if latest == nil {
		// No execution row recorded yet. The dispatcher may not have launched
		// this task — leave it alone, the scheduler/dispatcher owns it.
		return
	}
	switch latest.Status {
	case model.ExecStatusCompleted:
		if err := r.taskStore.CompleteTask(ctx, t.ID); err != nil {
			log.Error("reconciler: complete stale task", zap.String("taskID", t.ID), zap.String("execID", latest.ID), zap.Error(err))
			return
		}
		log.Warn("reconciler: marked stale running task completed",
			zap.String("taskID", t.ID), zap.String("execID", latest.ID))
	case model.ExecStatusFailed:
		if err := r.taskStore.FailTask(ctx, t.ID); err != nil {
			log.Error("reconciler: fail stale task", zap.String("taskID", t.ID), zap.String("execID", latest.ID), zap.Error(err))
			return
		}
		log.Warn("reconciler: marked stale running task failed",
			zap.String("taskID", t.ID), zap.String("execID", latest.ID))
	case model.ExecStatusRunning:
		// Exec also claims to be running. If its tracked PID is gone, the
		// process died without monitorExecution finalising it — sweep it.
		if latest.AIProcessPID <= 0 {
			return
		}
		if r.processAlive(latest.AIProcessPID) {
			return
		}
		if _, err := r.execStore.MarkAbandoned(ctx, latest.ID, "abandoned: process exited without completion signal"); err != nil {
			log.Error("reconciler: mark exec abandoned", zap.String("execID", latest.ID), zap.Error(err))
			return
		}
		if err := r.taskStore.FailTask(ctx, t.ID); err != nil {
			log.Error("reconciler: fail orphaned task", zap.String("taskID", t.ID), zap.String("execID", latest.ID), zap.Error(err))
			return
		}
		log.Warn("reconciler: swept orphaned running task",
			zap.String("taskID", t.ID), zap.String("execID", latest.ID), zap.Int("pid", latest.AIProcessPID))
	}
}

// latestExecution returns the most recent execution row for a task: the running
// one if present, otherwise the newest by start time. Returns (nil, nil) when
// the task has no executions yet.
func (r *Reconciler) latestExecution(ctx context.Context, taskID string) (*model.WorkerExecution, error) {
	if running, err := r.execStore.GetRunningByTaskID(ctx, taskID); err != nil {
		return nil, err
	} else if running != nil {
		return running, nil
	}
	byTask, err := r.execStore.ListByTaskIDs(ctx, []string{taskID}, 1)
	if err != nil {
		return nil, err
	}
	execs := byTask[taskID]
	if len(execs) == 0 {
		return nil, nil
	}
	latest := execs[0]
	return &latest, nil
}

