package ai

import (
	"bufio"
	"io"
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
	cmd *exec.Cmd
	mu  sync.Mutex
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
	var hosts []string
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.EqualFold(key, "no_proxy") {
			continue
		}
		for _, h := range strings.Split(value, ",") {
			if h = strings.TrimSpace(h); h != "" && !slices.Contains(hosts, h) {
				hosts = append(hosts, h)
			}
		}
	}
	if !slices.Contains(hosts, u.Hostname()) {
		hosts = append(hosts, u.Hostname())
	}
	merged := strings.Join(hosts, ",")
	keys := []string{"NO_PROXY", "no_proxy"}
	if runtime.GOOS == "windows" {
		keys = keys[:1]
	}
	for _, key := range keys {
		env = append(env, key+"="+merged)
	}
	return env
}

// AppendExtraEnv appends non-empty entries from extraEnv to base and returns
// the result re-clipped to its length, preventing concurrent Run() appends from
// sharing the backing array with other goroutines.
func AppendExtraEnv(base []string, extraEnv map[string]string) []string {
	for k, v := range extraEnv {
		if v != "" {
			base = append(base, k+"="+v)
		}
	}
	return base[:len(base):len(base)]
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

// ScanJSONLines reads r line by line and calls fn for each line that starts
// with '{'. fn returns true to keep scanning or false to stop early.
func ScanJSONLines(r io.Reader, fn func(line string) bool) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(nil, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "{") && !fn(line) {
			return
		}
	}
}
