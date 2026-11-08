//go:build !windows

package sysproc

import "os/exec"

func HideConsole(*exec.Cmd) {}
