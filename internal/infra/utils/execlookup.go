package utils

import (
	"context"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/model"
)

// RunningExecLookup resolves the running execution id for a set of tasks.
type RunningExecLookup interface {
	RunningExecIDsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]string, error)
}

// ErrorLogger is the minimal error-logging surface that both *zap.Logger and
// the project's logger.Logger satisfy.
type ErrorLogger interface {
	Error(msg string, fields ...zap.Field)
}

// RunningExecIDsForTasks returns an empty map on lookup error so callers can
// keep going without an exec id column.
func RunningExecIDsForTasks(ctx context.Context, logger ErrorLogger, lookup RunningExecLookup, tasks []model.Task, op string) map[string]string {
	if len(tasks) == 0 {
		return map[string]string{}
	}
	taskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		taskIDs = append(taskIDs, t.ID)
	}
	execIDs, err := lookup.RunningExecIDsByTaskIDs(ctx, taskIDs)
	if err != nil {
		logger.Error("resolve running exec ids", zap.String("op", op), zap.Error(err))
		return map[string]string{}
	}
	return execIDs
}
