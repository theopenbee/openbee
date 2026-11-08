//go:build embedserver

package localserver

import (
	"cmp"
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	pathMarker   = "__OPENBEE_PATH__"
	shellTimeout = 5 * time.Second
)

var errMarker = errors.New("the shell printed no PATH")

func importShellPath() {
	if runtime.GOOS == "windows" {
		return
	}
	shell := cmp.Or(os.Getenv("SHELL"), defaultShell())
	path, err := loginShellPath(shell, shellTimeout)
	if err != nil {
		log.Printf("localserver: read PATH from %s: %v", shell, err)
		return
	}
	os.Setenv("PATH", mergePaths(path, os.Getenv("PATH")))
}

func defaultShell() string {
	if runtime.GOOS == "darwin" {
		return "/bin/zsh"
	}
	return "/bin/sh"
}

func loginShellPath(shell string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-ilc", "echo "+pathMarker+"; printenv PATH; echo "+pathMarker)
	cmd.Env = append(os.Environ(), "DISABLE_AUTO_UPDATE=true")
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	_, rest, ok := strings.Cut(string(out), pathMarker)
	if !ok {
		return "", errMarker
	}
	path, _, ok := strings.Cut(rest, pathMarker)
	if !ok {
		return "", errMarker
	}
	return strings.TrimSpace(path), nil
}

func mergePaths(lists ...string) string {
	seen := map[string]bool{}
	var dirs []string
	for _, list := range lists {
		for _, dir := range filepath.SplitList(list) {
			if dir == "" || seen[dir] {
				continue
			}
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	return strings.Join(dirs, string(os.PathListSeparator))
}
