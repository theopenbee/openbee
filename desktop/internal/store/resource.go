package store

import (
	"context"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

type Resource[T any] struct {
	Data      T
	Loaded    bool
	Loading   bool
	Err       error
	FetchedAt time.Time

	s       *Store
	fetch   func(ctx context.Context, c *api.Client) (T, error)
	settled func()
	waiters []func()
	again   bool
	stale   bool
	seq     uint64
}

func NewResource[T any](s *Store, fetch func(ctx context.Context, c *api.Client) (T, error)) *Resource[T] {
	r := &Resource[T]{s: s, fetch: fetch}
	s.resets = append(s.resets, r.Reset)
	return r
}

func (r *Resource[T]) Ensure() {
	if r.Loading || (r.Loaded || r.Err != nil) && !r.stale {
		return
	}
	r.load()
}

func (r *Resource[T]) markStale() {
	if r.Loaded || r.Loading || r.Err != nil {
		r.stale = true
	}
}

func (r *Resource[T]) Invalidate() {
	switch {
	case r.Loading:
		r.again = true
	case r.Loaded || r.Err != nil:
		r.load()
	}
}

func (r *Resource[T]) Set(v T) {
	r.seq++
	r.Data = v
	r.Loaded = true
	r.Loading = false
	r.Err = nil
	r.FetchedAt = r.s.o.Now()
	r.again = false
	r.stale = false
	r.flushWaiters()
}

func (r *Resource[T]) Reset() {
	var zero T
	r.seq++
	r.Data = zero
	r.Loaded = false
	r.Loading = false
	r.Err = nil
	r.FetchedAt = time.Time{}
	r.again = false
	r.stale = false
	r.waiters = nil
}

func (r *Resource[T]) Stale(d time.Duration) bool {
	return r.FetchedAt.IsZero() || r.s.o.Now().Sub(r.FetchedAt) >= d
}

func (r *Resource[T]) patch(v T) {
	r.Data = v
	r.FetchedAt = r.s.o.Now()
}

func (r *Resource[T]) refetch(then func()) {
	r.waiters = append(r.waiters, then)
	if !r.Loading {
		r.load()
	}
}

func (r *Resource[T]) reload(then func()) {
	r.waiters = append(r.waiters, then)
	r.load()
}

func (r *Resource[T]) flushWaiters() {
	waiters := r.waiters
	r.waiters = nil
	for _, fn := range waiters {
		fn()
	}
}

func (r *Resource[T]) load() {
	r.seq++
	seq := r.seq
	r.Loading = true
	r.again = false
	r.stale = false
	fetch := r.fetch
	r.s.async(func(ctx context.Context, c *api.Client) func() {
		v, err := fetch(ctx, c)
		return func() {
			if r.seq != seq {
				return
			}
			r.Loading = false
			if err != nil {
				r.Err = err
			} else {
				r.Data = v
				r.Loaded = true
				r.Err = nil
				r.FetchedAt = r.s.o.Now()
			}
			if r.settled != nil {
				r.settled()
			}
			r.flushWaiters()
			if r.again {
				r.load()
			}
		}
	}, nil)
}
