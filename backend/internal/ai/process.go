package ai

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
)

// CmdProcess implements Process for an os/exec.Cmd.
type CmdProcess struct {
	cmd     *exec.Cmd
	mu      sync.Mutex
	stopped bool
}

// NewCmdProcess wraps an exec.Cmd as a Process.
func NewCmdProcess(cmd *exec.Cmd) *CmdProcess {
	return &CmdProcess{cmd: cmd}
}

func (p *CmdProcess) PID() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// BuildRunEnv assembles the final env slice for a subprocess run.
// Entries are ordered baseEnv → extraEnv → apiKey; for duplicate keys the
// last value wins (standard subprocess env resolution on Linux/macOS), so
// extraEnv overrides baseEnv and apiKey overrides both.
func BuildRunEnv(baseEnv, extraEnv []string, apiKey string) []string {
	env := make([]string, 0, len(baseEnv)+len(extraEnv)+1)
	env = append(env, baseEnv...)
	env = append(env, extraEnv...)
	env = append(env, APIKeyEnv+"="+apiKey)
	return bypassProxyForMCP(env, MCPURL())
}

func bypassProxyForMCP(env []string, mcpURL string) []string {
	u, err := url.Parse(mcpURL)
	if mcpURL == "" || err != nil || u.Hostname() == "" {
		return env
	}
	keys := []string{"NO_PROXY", "no_proxy"}
	if runtime.GOOS == "windows" {
		keys = keys[:1]
	}
	var hosts []string
	for _, key := range keys {
		for _, h := range strings.Split(effectiveEnv(env, key), ",") {
			h = strings.TrimSpace(h)
			if h == "*" {
				return env
			}
			if h != "" && !slices.Contains(hosts, h) {
				hosts = append(hosts, h)
			}
		}
	}
	if !slices.Contains(hosts, u.Hostname()) {
		hosts = append(hosts, u.Hostname())
	}
	merged := strings.Join(hosts, ",")
	for _, key := range keys {
		env = append(env, key+"="+merged)
	}
	return env
}

func effectiveEnv(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		k, v, ok := strings.Cut(env[i], "=")
		if ok && (k == key || runtime.GOOS == "windows" && strings.EqualFold(k, key)) {
			return v
		}
	}
	return ""
}

// BuildBaseEnv constructs the base environment for engine subprocesses.
// It prepends the current executable's directory to PATH.
// OPENBEE_URL is inherited from the server process environment (set via os.Setenv at startup).
func BuildBaseEnv() []string {
	sysEnv := os.Environ()
	env := make([]string, 0, len(sysEnv)+1)
	if exePath, err := os.Executable(); err == nil {
		oldPath := os.Getenv("PATH")
		for _, e := range sysEnv {
			if !strings.HasPrefix(e, "PATH=") {
				env = append(env, e)
			}
		}
		env = append(env, "PATH="+filepath.Dir(exePath)+string(os.PathListSeparator)+oldPath)
	} else {
		env = append(env, sysEnv...)
	}
	// Clip to length so concurrent append calls in Run() cannot share the backing array.
	return env[:len(env):len(env)]
}
