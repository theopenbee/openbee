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
	UpdateStatusIfRunning(ctx context.Context, taskID, next string) (bool, error)
}

// reconcilerExecStore is the subset of store.ExecutionStore used by the Reconciler.
type reconcilerExecStore interface {
	ListByTaskIDs(ctx context.Context, taskIDs []string, limitPerTask int) (map[string][]model.WorkerExecution, error)
	RunningExecIDsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]string, error)
	MarkAbandoned(ctx context.Context, id, result string) (bool, error)
}

// Reconciler periodically repairs tasks whose status drifted out of sync with
// their underlying execution row (engine adapter bugs, OS-killed workers, etc.)
// so /status reflects the actual state without waiting for a server restart.
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
	if len(tasks) == 0 {
		return
	}
	taskIDs := make([]string, len(tasks))
	for i, t := range tasks {
		taskIDs[i] = t.ID
	}
	runningIDs, err := r.execStore.RunningExecIDsByTaskIDs(ctx, taskIDs)
	if err != nil {
		log.Error("reconciler: running exec ids", zap.Error(err))
		return
	}
	latestByTask, err := r.execStore.ListByTaskIDs(ctx, taskIDs, 1)
	if err != nil {
		log.Error("reconciler: list executions", zap.Error(err))
		return
	}
	for _, t := range tasks {
		select {
		case <-ctx.Done():
			return
		default:
		}
		latest := pickLatest(latestByTask[t.ID], runningIDs[t.ID])
		if latest == nil {
			continue
		}
		r.applyDecision(ctx, t, *latest)
	}
}

// pickLatest selects the execution row the reconciler should act on. When a
// running exec ID is known, the matching row is returned so liveness probing
// targets the currently-claimed PID; otherwise the most recent row wins.
func pickLatest(latest []model.WorkerExecution, runningID string) *model.WorkerExecution {
	if runningID != "" {
		for i := range latest {
			if latest[i].ID == runningID {
				return &latest[i]
			}
		}
		// Running row exists but isn't in the latest snapshot (race with
		// concurrent insert). Skip this tick; the next will catch it.
		return nil
	}
	if len(latest) == 0 {
		return nil
	}
	return &latest[0]
}

func (r *Reconciler) applyDecision(ctx context.Context, t model.Task, latest model.WorkerExecution) {
	switch latest.Status {
	case model.ExecStatusCompleted:
		r.transition(ctx, t.ID, latest.ID, model.TaskStatusCompleted, "completed")
	case model.ExecStatusFailed:
		r.transition(ctx, t.ID, latest.ID, model.TaskStatusFailed, "failed")
	case model.ExecStatusRunning:
		// Exec also claims to be running. If its tracked PID is gone, the
		// process died without monitorExecution finalising it — sweep it.
		if latest.AIProcessPID <= 0 || r.processAlive(latest.AIProcessPID) {
			return
		}
		if _, err := r.execStore.MarkAbandoned(ctx, latest.ID, "abandoned: process exited without completion signal"); err != nil {
			log.Error("reconciler: mark exec abandoned", zap.String("execID", latest.ID), zap.Error(err))
			return
		}
		changed, err := r.taskStore.UpdateStatusIfRunning(ctx, t.ID, model.TaskStatusFailed)
		if err != nil {
			log.Error("reconciler: fail orphaned task", zap.String("taskID", t.ID), zap.String("execID", latest.ID), zap.Error(err))
			return
		}
		if changed {
			log.Warn("reconciler: swept orphaned running task",
				zap.String("taskID", t.ID), zap.String("execID", latest.ID), zap.Int("pid", latest.AIProcessPID))
		}
	}
}

func (r *Reconciler) transition(ctx context.Context, taskID, execID, next, label string) {
	changed, err := r.taskStore.UpdateStatusIfRunning(ctx, taskID, next)
	if err != nil {
		log.Error("reconciler: "+label+" stale task",
			zap.String("taskID", taskID), zap.String("execID", execID), zap.Error(err))
		return
	}
	if changed {
		log.Warn("reconciler: marked stale running task "+label,
			zap.String("taskID", taskID), zap.String("execID", execID))
	}
}
