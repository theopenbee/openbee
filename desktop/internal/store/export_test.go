package store

import (
	"maps"
	"slices"
)

func Generation(s *Store) uint64 { return s.sess.gen }

func CachedRuns(s *Store) []string { return slices.Sorted(maps.Keys(s.executions)) }

func ResetHooks(s *Store) int { return len(s.resets) }
