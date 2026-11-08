//go:build windows

package sysproc

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestHideConsoleKeepsOtherFlags(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	HideConsole(cmd)
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow not set")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 || cmd.SysProcAttr.CreationFlags&syscall.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Fatalf("CreationFlags = %#x, want CREATE_NO_WINDOW added to the existing flags", cmd.SysProcAttr.CreationFlags)
	}
}
