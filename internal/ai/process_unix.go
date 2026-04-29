//go:build !windows

package ai

import (
	"os/exec"
	"syscall"
)

// Requires that ConfigureCmd was called on the underlying cmd before Start.
func (p *CmdProcess) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}

// ConfigureCmd puts the engine subprocess in its own process group so Stop() can kill
// the entire group, including child processes the engine spawns (e.g. bash tool subprocesses).
func ConfigureCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
