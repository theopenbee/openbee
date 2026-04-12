package ai

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// CmdProcess implements Process for an os/exec.Cmd.
type CmdProcess struct {
	cmd *exec.Cmd
	mu  sync.Mutex
}

// NewCmdProcess wraps an exec.Cmd as a Process.
func NewCmdProcess(cmd *exec.Cmd) *CmdProcess {
	return &CmdProcess{cmd: cmd}
}

func (p *CmdProcess) PID() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

func (p *CmdProcess) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

// BuildBaseEnv constructs the base environment for engine subprocesses.
// It prepends the current executable's directory to PATH and appends OPENBEE_URL.
func BuildBaseEnv(openbeeURL string) []string {
	sysEnv := os.Environ()
	env := make([]string, 0, len(sysEnv)+2)
	if exePath, err := os.Executable(); err == nil {
		oldPath := os.Getenv("PATH")
		for _, e := range sysEnv {
			if !strings.HasPrefix(e, "PATH=") {
				env = append(env, e)
			}
		}
		env = append(env, "PATH="+filepath.Dir(exePath)+string(os.PathListSeparator)+oldPath)
	} else {
		env = append(env, sysEnv...)
	}
	env = append(env, "OPENBEE_URL="+openbeeURL)
	// Clip to length so concurrent append calls in Run() cannot share the backing array.
	return env[:len(env):len(env)]
}
