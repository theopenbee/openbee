package daemoncmd

// This file exposes a minimal set of helpers that the remaining package-main
// commands (backup, restore) use until they are migrated in Tasks 5-7.
// These exports should be removed once all callers have been moved.

// OpenbeeStateDir returns ~/.openbee. Exported for use by restore/backup commands
// while they still live in package main.
func OpenbeeStateDir() string { return openbeeStateDir() }

// DaemonPIDFile returns the path to the PID file. Exported temporarily.
func DaemonPIDFile() string { return daemonPIDFile() }

// DoStop is the exported wrapper around doStop for use by restore.go.
func DoStop(pidFile string) error { return doStop(pidFile) }
