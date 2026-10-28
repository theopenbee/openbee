//go:build windows

package ai

import (
	"os/exec"
	"strconv"
)

func (p *CmdProcess) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	pid := p.cmd.Process.Pid
	kill := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
	return kill.Run()
}

func ConfigureCmd(cmd *exec.Cmd) {}
