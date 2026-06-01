//go:build !windows

package utils

import "syscall"

// IsProcessAlive reports whether a process with the given PID is currently running.
// Uses kill(pid, 0) — the zero-signal POSIX liveness probe.
// EPERM (process exists but owned by another user) is treated as "not alive" so
// we never signal a process we do not own.
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}
