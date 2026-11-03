package store

import (
	"context"
	"slices"
	"sync"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func (s *Store) SaveSystemConfigs(changes map[string]string, done func(error)) {
	if done == nil {
		done = func(error) {}
	}
	if len(changes) == 0 {
		done(nil)
		return
	}
	s.async(func(ctx context.Context, c *api.Client) func() {
		err := putSystemConfigs(ctx, c, changes)
		return func() {
			r := s.SystemConfigs
			r.waiters = append(r.waiters, func() { done(err) })
			r.load()
		}
	}, func() { done(context.Canceled) })
}

func putSystemConfigs(ctx context.Context, c *api.Client, changes map[string]string) error {
	keys := make([]string, 0, len(changes))
	for key := range changes {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	errs := make([]error, len(keys))
	var wg sync.WaitGroup
	for i, key := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = c.SetSystemConfig(ctx, key, changes[key])
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
