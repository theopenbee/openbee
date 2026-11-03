package utils

import "strconv"

// DerefStr dereferences a *string, returning "<nil>" for nil pointers (useful for logging).
func DerefStr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

// DerefStrOrEmpty dereferences a *string, returning "" for nil pointers.
func DerefStrOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ParseMillis converts a *string millisecond timestamp (e.g. "1609073151345") to int64.
// Returns 0 for nil or unparseable input.
func ParseMillis(s *string) int64 {
	if s == nil {
		return 0
	}
	v, err := strconv.ParseInt(*s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}
