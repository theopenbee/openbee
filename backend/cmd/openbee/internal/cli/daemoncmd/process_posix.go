//go:build darwin || linux

package daemoncmd

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// redirectStdio replaces OS file descriptors 1 and 2 with the given log file.
// Uses unix.Dup2 (which wraps dup3 on Linux). Closes lf after duplicating so
// fd 1 and fd 2 are the sole holders of the log file descriptor.
// Must be called before logger.Init.
func redirectStdio(logPath string) error {
	lf, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", logPath, err)
	}
	fd := int(lf.Fd())
	if err := unix.Dup2(fd, 1); err != nil {
		lf.Close()
		return fmt.Errorf("dup stdout: %w", err)
	}
	if err := unix.Dup2(fd, 2); err != nil {
		lf.Close()
		return fmt.Errorf("dup stderr: %w", err)
	}
	lf.Close()
	os.Stdout = os.NewFile(1, "/dev/stdout")
	os.Stderr = os.NewFile(2, "/dev/stderr")
	return nil
}
