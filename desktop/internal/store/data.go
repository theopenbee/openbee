package store

import (
	"context"
	"slices"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/model"
)

const staleAfter = 30 * time.Second

func (s *Store) initData() {
	s.Workers = NewResource(s, func(ctx context.Context, c *api.Client) ([]api.Worker, error) {
		return c.Workers(ctx)
	})
	s.Conversations = NewResource(s, func(ctx context.Context, c *api.Client) ([]api.ConversationSummary, error) {
		return c.Conversations(ctx)
	})
	s.Conversations.settled = s.syncUnread
	s.SystemConfigs = NewResource(s, func(ctx context.Context, c *api.Client) (map[string]string, error) {
		return c.SystemConfigs(ctx)
	})
	s.details = map[string]*Resource[api.Worker]{}
	s.envs = map[envScope]*Resource[[]api.EnvConfig]{}
	s.initPlugins()
}

func (s *Store) WorkerMap() map[string]api.Worker {
	if !s.Workers.Loaded {
		return nil
	}
	m := make(map[string]api.Worker, len(s.Workers.Data))
	for _, w := range s.Workers.Data {
		m[w.ID] = w
	}
	return m
}

func (s *Store) Worker(id string) (api.Worker, bool) {
	i := s.workerIndex(id)
	if i < 0 {
		return api.Worker{}, false
	}
	return s.Workers.Data[i], true
}

func (s *Store) WorkerDetail(id string) *Resource[api.Worker] {
	if r, ok := s.details[id]; ok {
		return r
	}
	r := NewResource(s, func(ctx context.Context, c *api.Client) (api.Worker, error) {
		return c.Worker(ctx, id)
	})
	s.details[id] = r
	return r
}

func (s *Store) EnabledEngines() []string {
	s.Config.Ensure()
	return model.EnabledEngines(s.Config.Data)
}

func (s *Store) workerIndex(id string) int {
	if !s.Workers.Loaded {
		return -1
	}
	return slices.IndexFunc(s.Workers.Data, func(w api.Worker) bool { return w.ID == id })
}

func (s *Store) summaries() []api.ConversationSummary {
	if !s.Conversations.Loaded {
		return nil
	}
	return s.Conversations.Data
}

func (s *Store) invalidateWorkers() {
	s.Workers.Invalidate()
	for _, r := range s.details {
		r.Invalidate()
	}
}

func (s *Store) patchWorkerStatus(id, status string) {
	if i := s.workerIndex(id); i >= 0 && s.Workers.Data[i].Status != status {
		workers := slices.Clone(s.Workers.Data)
		workers[i].Status = status
		s.Workers.patch(workers)
	}
	if r, ok := s.details[id]; ok && r.Loaded && r.Data.Status != status {
		w := r.Data
		w.Status = status
		r.patch(w)
	}
}

func (s *Store) refetchStale() {
	refetchIfStale(s.Workers)
	for _, r := range s.details {
		refetchIfStale(r)
	}
	refetchIfStale(s.Conversations)
}

func refetchIfStale[T any](r *Resource[T]) {
	if !r.Loading && r.Stale(staleAfter) {
		r.Invalidate()
	}
}
