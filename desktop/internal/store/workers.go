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

func (s *Store) CreateWorker(req api.CreateWorkerRequest, done func(api.Worker, error)) {
	if done == nil {
		done = func(api.Worker, error) {}
	}
	s.async(func(ctx context.Context, c *api.Client) func() {
		w, err := c.CreateWorker(ctx, req)
		return func() {
			if err == nil {
				s.Workers.Invalidate()
				s.Conversations.Invalidate()
			}
			done(w, err)
		}
	}, func() { done(api.Worker{}, context.Canceled) })
}

func (s *Store) DeleteWorker(id string, deleteWorkDir bool, done func(error)) {
	if done == nil {
		done = func(error) {}
	}
	s.async(func(ctx context.Context, c *api.Client) func() {
		err := c.DeleteWorker(ctx, id, deleteWorkDir)
		return func() {
			if err == nil {
				delete(s.details, id)
				delete(s.chats, api.WorkerConversation(id))
				delete(s.envs, envScope{scope: envScopeWorker, scopeID: id})
				s.invalidateWorkers()
				s.Conversations.Invalidate()
			}
			done(err)
		}
	}, func() { done(context.Canceled) })
}
