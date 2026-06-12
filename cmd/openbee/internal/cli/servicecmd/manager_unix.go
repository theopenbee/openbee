//go:build darwin || linux

package servicecmd

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
)

var execLookPath = exec.LookPath

func readUptime(ctx context.Context, pid int) int64 {
	out, err := runCommand(ctx, "ps", "-o", "etimes=", "-p", strconv.Itoa(pid))
	if err != nil {
		return 0
	}
	if v, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); err == nil {
		return v
	}
	return 0
}
