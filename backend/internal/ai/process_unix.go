//go:build !windows

package ai

import (
	"os/exec"
	"syscall"
	"time"
)

const terminatePollInterval = 50 * time.Millisecond

// Requires that ConfigureCmd was called on the underlying cmd before Start.
func (p *CmdProcess) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return err
	}
	p.stopped = true
	return nil
}

func (p *CmdProcess) Terminate(grace time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped || p.terminating || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	pgid := p.cmd.Process.Pid
	err := syscall.Kill(-pgid, syscall.SIGTERM)
	if err == syscall.ESRCH {
		p.stopped = true
		return nil
	}
	if err != nil && err != syscall.EPERM {
		return err
	}
	p.terminating = true
	terminated := make(chan struct{})
	p.terminated = terminated
	go func() {
		defer close(terminated)
		killGroupAfter(pgid, grace)
	}()
	return nil
}

func killGroupAfter(pgid int, grace time.Duration) {
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if syscall.Kill(-pgid, 0) == syscall.ESRCH {
			return
		}
		time.Sleep(min(terminatePollInterval, time.Until(deadline)))
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}

// ConfigureCmd puts the engine subprocess in its own process group so Stop() can kill
// the entire group, including child processes the engine spawns (e.g. bash tool subprocesses).
func ConfigureCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
