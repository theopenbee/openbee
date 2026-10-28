package store

import (
	"context"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func (s *Store) UpdateWorker(id string, req api.UpdateWorkerRequest, done func(api.Worker, error)) {
	if done == nil {
		done = func(api.Worker, error) {}
	}
	s.async(func(ctx context.Context, c *api.Client) func() {
		w, err := c.UpdateWorker(ctx, id, req)
		return func() {
			if err == nil {
				s.Workers.Invalidate()
				if r, ok := s.details[id]; ok {
					r.Invalidate()
				}
			}
			done(w, err)
		}
	}, func() { done(api.Worker{}, context.Canceled) })
}

func (s *Store) RandomName(done func(api.RandomName, error)) {
	if done == nil {
		done = func(api.RandomName, error) {}
	}
	s.async(func(ctx context.Context, c *api.Client) func() {
		name, err := c.RandomWorkerName(ctx)
		return func() { done(name, err) }
	}, func() { done(api.RandomName{}, context.Canceled) })
}
