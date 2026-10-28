package store

import "github.com/theopenbee/openbee/desktop/internal/kit"

func (s *Store) Toast(t kit.Toast) {
	s.Toasts = append(s.Toasts, t)
}
