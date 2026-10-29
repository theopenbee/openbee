package store

import (
	"context"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

const envScopeWorker = "worker"

type envScope struct{ scope, scopeID string }

func (s *Store) Envs(scope, scopeID string) *Resource[[]api.EnvConfig] {
	key := envScope{scope, scopeID}
	if r, ok := s.envs[key]; ok {
		return r
	}
	r := NewResource(s, func(ctx context.Context, c *api.Client) ([]api.EnvConfig, error) {
		return c.Envs(ctx, scope, scopeID)
	})
	s.envs[key] = r
	return r
}

func (s *Store) CreateEnv(req api.CreateEnvRequest, done func(error)) {
	s.mutateEnv(req.Scope, req.ScopeID, done, func(ctx context.Context, c *api.Client) error {
		_, err := c.CreateEnv(ctx, req)
		return err
	})
}

func (s *Store) UpdateEnv(scope, scopeID, id, value string, done func(error)) {
	s.mutateEnv(scope, scopeID, done, func(ctx context.Context, c *api.Client) error {
		return c.UpdateEnv(ctx, id, value)
	})
}

func (s *Store) DeleteEnv(scope, scopeID, id string, done func(error)) {
	s.mutateEnv(scope, scopeID, done, func(ctx context.Context, c *api.Client) error {
		return c.DeleteEnv(ctx, id)
	})
}

func (s *Store) mutateEnv(scope, scopeID string, done func(error), do func(ctx context.Context, c *api.Client) error) {
	if done == nil {
		done = func(error) {}
	}
	s.async(func(ctx context.Context, c *api.Client) func() {
		err := do(ctx, c)
		return func() {
			if err == nil {
				s.Envs(scope, scopeID).Invalidate()
			}
			done(err)
		}
	}, func() { done(context.Canceled) })
}
