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

func (f fakeRunningExecs) RunningExecIDsByTaskIDs(_ context.Context, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		if v, ok := f[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

// execIDsFromTasks is kept for call-site compatibility; it now returns an empty map
// since ExecutionID has been removed from model.Task. Call sites that need running-exec
// entries should construct fakeRunningExecs directly.
func execIDsFromTasks(_ []model.Task) fakeRunningExecs {
	return fakeRunningExecs{}
}
