package command_test

import (
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
