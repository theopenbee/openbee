package config

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	homeOnce sync.Once
	homeDir  string
	homeErr  error
)

// OpenbeeHomeDir returns ~/.openbee, caching the resolved path across calls.
func OpenbeeHomeDir() (string, error) {
	homeOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			homeErr = err
			return
		}
		homeDir = filepath.Join(home, ".openbee")
	})
	return homeDir, homeErr
}

// DaemonPIDFile returns the daemon PID file path under OpenbeeHomeDir.
func DaemonPIDFile() (string, error) {
	h, err := OpenbeeHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, "openbee.pid"), nil
}

// DaemonLogFile returns the daemon log file path under OpenbeeHomeDir.
func DaemonLogFile() (string, error) {
	h, err := OpenbeeHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, "openbee.log"), nil
}
