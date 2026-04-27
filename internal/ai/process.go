package ai

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

func (p *CmdProcess) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

// BuildRunEnv assembles the final env slice for a subprocess run.
// Entries are ordered baseEnv → extraEnv → apiKey; for duplicate keys the
// last value wins (standard subprocess env resolution on Linux/macOS), so
// extraEnv overrides baseEnv and apiKey overrides both.
func BuildRunEnv(baseEnv, extraEnv []string, apiKey string) []string {
	env := make([]string, 0, len(baseEnv)+len(extraEnv)+1)
	env = append(env, baseEnv...)
	env = append(env, extraEnv...)
	env = append(env, "OPENBEE_API_KEY="+apiKey)
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
// It prepends the current executable's directory to PATH and appends OPENBEE_URL.
func BuildBaseEnv(openbeeURL string) []string {
	sysEnv := os.Environ()
	env := make([]string, 0, len(sysEnv)+2)
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
	env = append(env, "OPENBEE_URL="+openbeeURL)
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
