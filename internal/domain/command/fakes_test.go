package command_test

import (
	"context"

	"github.com/theopenbee/openbee/internal/infra/model"
)

type fakeWorkerByIDsLookup struct {
	byID map[string]model.Worker
	err  error
}

func (f *fakeWorkerByIDsLookup) GetByIDs(ids []string) ([]model.Worker, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]model.Worker, 0, len(ids))
	for _, id := range ids {
		if w, ok := f.byID[id]; ok {
			out = append(out, w)
		}
	}
	return out, nil
}

// fakeRunningExecs implements RunningExecLookup for tests.
// It maps task_id -> execution_id.
type fakeRunningExecs map[string]string

func (f fakeRunningExecs) RunningExecIDsByTaskIDs(_ context.Context, _ []string) (map[string]string, error) {
	return map[string]string(f), nil
}

// execIDsFromTasks builds a fakeRunningExecs from a slice of tasks using each
// task's ExecutionID field (keyed by task ID). Tasks with empty ExecutionID are omitted.
func execIDsFromTasks(tasks []model.Task) fakeRunningExecs {
	m := make(fakeRunningExecs, len(tasks))
	for _, t := range tasks {
		if t.ExecutionID != "" {
			m[t.ID] = t.ExecutionID
		}
	}
	return m
}
