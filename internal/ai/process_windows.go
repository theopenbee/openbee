//go:build windows

package ai

import (
	"fmt"
	"os/exec"
)

// Stop kills the process tree on Windows using taskkill /F /T /PID.
// This terminates the process and all child processes it spawned.
func (p *CmdProcess) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	pid := p.cmd.Process.Pid
	kill := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
	return kill.Run()
}

// ConfigureCmd is a no-op on Windows; process tree termination is handled by Stop via taskkill.
func ConfigureCmd(cmd *exec.Cmd) {}
