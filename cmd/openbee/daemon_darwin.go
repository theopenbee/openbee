//go:build darwin

package main

import (
	"fmt"
	"os"
	"syscall"
)

// redirectStdio replaces OS file descriptors 1 and 2 with the given log file.
// Uses syscall.Dup2 (available on macOS). Closes lf after duplicating.
// Must be called before logger.Init.
func redirectStdio(logPath string) error {
	lf, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", logPath, err)
	}
	fd := int(lf.Fd())
	if err := syscall.Dup2(fd, 1); err != nil {
		lf.Close()
		return fmt.Errorf("dup2 stdout: %w", err)
	}
	if err := syscall.Dup2(fd, 2); err != nil {
		lf.Close()
		return fmt.Errorf("dup2 stderr: %w", err)
	}
	lf.Close()
	os.Stdout = os.NewFile(1, "/dev/stdout")
	os.Stderr = os.NewFile(2, "/dev/stderr")
	return nil
}
