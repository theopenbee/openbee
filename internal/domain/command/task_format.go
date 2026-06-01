package command

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

type WorkerByIDsLookup interface {
	GetByIDs(ids []string) ([]model.Worker, error)
}

// RunningExecLookup resolves the running execution id for a set of tasks.
type RunningExecLookup interface {
	RunningExecIDsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]string, error)
}

// runningExecIDsForTasks returns an empty map on lookup error so callers can
// keep formatting without an exec id column.
func runningExecIDsForTasks(ctx context.Context, lookup RunningExecLookup, tasks []model.Task, op string) map[string]string {
	if len(tasks) == 0 {
		return map[string]string{}
	}
	taskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		taskIDs = append(taskIDs, t.ID)
	}
	execIDs, err := lookup.RunningExecIDsByTaskIDs(ctx, taskIDs)
	if err != nil {
		log.Error("resolve running exec ids", zap.String("op", op), zap.Error(err))
		return map[string]string{}
	}
	return execIDs
}

func formatTaskLine(format string, t model.Task, workerNames, execIDs map[string]string, nowMs int64) string {
	runtimeSec := (nowMs - t.CreatedAt) / 1000
	return fmt.Sprintf(format,
		workerNameOrFallback(workerNames, t.WorkerID),
		utils.TruncateRunes(strings.Join(strings.Fields(t.Instruction), " "), maxInstructionRunes),
		formatRelative(runtimeSec),
		shortExecID(execIDs[t.ID]),
	)
}

// resolveWorkerNames returns a {workerID -> name} map for the workers
// referenced by tasks. Returns nil on lookup error so callers fall back to
// raw IDs via workerNameOrFallback.
func resolveWorkerNames(workers WorkerByIDsLookup, tasks []model.Task) map[string]string {
	if len(tasks) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tasks))
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		if t.WorkerID == "" {
			continue
		}
		if _, ok := seen[t.WorkerID]; ok {
			continue
		}
		seen[t.WorkerID] = struct{}{}
		ids = append(ids, t.WorkerID)
	}
	if len(ids) == 0 {
		return nil
	}
	ws, err := workers.GetByIDs(ids)
	if err != nil {
		log.Error("batch lookup workers", zap.Error(err))
		return nil
	}
	out := make(map[string]string, len(ws))
	for _, w := range ws {
		if w.Name != "" {
			out[w.ID] = w.Name
		}
	}
	return out
}
