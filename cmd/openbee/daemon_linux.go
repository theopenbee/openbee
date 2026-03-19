//go:build linux

package main

import (
	"fmt"
	"os"
	"syscall"
)

// redirectStdio replaces OS file descriptors 1 and 2 with the given log file.
// Uses syscall.Dup3 (Linux-only, all arches including arm64). Closes lf after
// duplicating so fd 1 and fd 2 are the sole holders of the log file descriptor.
// Must be called before logger.Init.
func redirectStdio(logPath string) error {
	lf, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", logPath, err)
	}
	fd := int(lf.Fd())
	if err := syscall.Dup3(fd, 1, 0); err != nil {
		lf.Close()
		return fmt.Errorf("dup3 stdout: %w", err)
	}
	if err := syscall.Dup3(fd, 2, 0); err != nil {
		lf.Close()
		return fmt.Errorf("dup3 stderr: %w", err)
	}
	lf.Close()
	return nil
}
