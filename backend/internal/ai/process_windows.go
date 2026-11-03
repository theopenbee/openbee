//go:build windows

package ai

import (
	"os/exec"
	"strconv"
	"time"
)

func (p *CmdProcess) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	pid := p.cmd.Process.Pid
	kill := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
	if err := kill.Run(); err != nil {
		return err
	}
	p.stopped = true
	return nil
}

func (p *CmdProcess) Terminate(time.Duration) error {
	return p.Stop()
}

func ConfigureCmd(cmd *exec.Cmd) {}
