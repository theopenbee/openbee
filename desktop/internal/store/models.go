package store

import (
	"context"
	"slices"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

type modelsKey struct{ engine, workerID string }

func (s *Store) EngineModels(engine, workerID string) *Resource[[]api.EngineModel] {
	key := modelsKey{engine, workerID}
	if r, ok := s.models[key]; ok {
		return r
	}
	r := NewResource(s, func(ctx context.Context, c *api.Client) ([]api.EngineModel, error) {
		models, err := c.EngineModels(ctx, engine, workerID)
		return uniqueModels(models), err
	})
	s.models[key] = r
	return r
}

func uniqueModels(models []api.EngineModel) []api.EngineModel {
	seen := make(map[string]bool, len(models))
	return slices.DeleteFunc(models, func(m api.EngineModel) bool {
		if seen[m.ID] {
			return true
		}
		seen[m.ID] = true
		return false
	})
}

func (s *Store) staleModels() {
	for _, r := range s.models {
		r.markStale()
	}
}

func (s *Store) forgetModels(workerID string) {
	for key := range s.models {
		if key.workerID == workerID {
			delete(s.models, key)
		}
	}
}
