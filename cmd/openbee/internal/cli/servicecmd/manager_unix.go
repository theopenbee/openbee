//go:build darwin || linux

package servicecmd

import (
	"os/exec"
	"strconv"
	"strings"
)

var execLookPath = exec.LookPath

func readUptime(pid int) int64 {
	out, err := exec.Command("ps", "-o", "etimes=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	if v, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); err == nil {
		return v
	}
	return 0
}
