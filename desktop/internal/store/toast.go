package store

import (
	"slices"

	"github.com/theopenbee/openbee/desktop/internal/kit"
)

func (s *Store) Toast(t kit.Toast) {
	t.At = s.o.Now()
	s.Toasts = append(s.Toasts, t)
}

func (s *Store) PruneToasts() {
	now := s.o.Now()
	s.Toasts = slices.DeleteFunc(s.Toasts, func(t kit.Toast) bool {
		return !t.At.IsZero() && now.Sub(t.At) >= kit.ToastTimeout
	})
}
