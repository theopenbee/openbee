package command

import (
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

type WorkerByIDsLookup interface {
	GetByIDs(ids []string) ([]model.Worker, error)
}

// formatTaskLine renders one row using i18n.StatusCommand.TaskLine. Shared
// by /status and /clear so any column change lands in one place.
func formatTaskLine(t model.Task, workerNames map[string]string, nowMs int64) string {
	runtimeSec := (nowMs - t.CreatedAt) / 1000
	return fmt.Sprintf(i18n.M.Runtime.StatusCommand.TaskLine,
		workerNameOrFallback(workerNames, t.WorkerID),
		utils.TruncateRunes(strings.Join(strings.Fields(t.Instruction), " "), maxInstructionRunes),
		formatRelative(runtimeSec),
		shortExecID(t.ExecutionID),
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
