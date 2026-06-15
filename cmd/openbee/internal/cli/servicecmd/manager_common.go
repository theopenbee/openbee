package servicecmd

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
)

var runCommand = defaultRunCommand

func defaultRunCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// runWithExitCode returns the exit code of the child process alongside any
// transport-level error (process failed to start, ctx cancelled, etc.). It
// exists as its own injectable hook because callers care about the difference
// between "ran and returned non-zero" (a signal we map to a status enum) and
// "could not run at all" (an unknown).
var runWithExitCode = defaultRunWithExitCode

func defaultRunWithExitCode(ctx context.Context, name string, args ...string) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}

// lookupRunAsEnvPath returns the PATH the given user sees in a login shell.
// Non-Linux builds and the unsupported-OS build leave this as a no-op; the
// linux backend overrides it via init() to shell out to `runuser`. Returning
// ("", nil) signals the caller to fall back to the installer's PATH.
var lookupRunAsEnvPath = func(_ context.Context, _ string) (string, error) {
	return "", nil
}

// verifyNodeForRunAsUser reports whether `node` is reachable AND executable for
// the given user using the given PATH. Result codes:
//   - nodeCheckOK:           node found and the user can exec it
//   - nodeCheckMissing:      node not on PATH at all
//   - nodeCheckNotExecutable: node found but execve would fail (EACCES) — this
//     is the exact failure the runtime "/usr/bin/env: 'node': Permission denied"
//     error surfaces in the chat path.
//   - nodeCheckUnknown:      we could not run the check (helper not implemented,
//     `runuser` missing, etc.); fall back to a coarser warning.
var verifyNodeForRunAsUser = func(_ context.Context, _, _ string) nodeCheckResult {
	return nodeCheckUnknown
}

type nodeCheckResult int

const (
	nodeCheckUnknown nodeCheckResult = iota
	nodeCheckOK
	nodeCheckMissing
	nodeCheckNotExecutable
)

func runOrWrap(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := runCommand(ctx, name, args...)
	if err != nil {
		return out, wrapRunErr(name, args, err, out)
	}
	return out, nil
}

func wrapRunErr(name string, args []string, err error, out []byte) error {
	label := name
	if len(args) > 0 {
		label = name + " " + args[0]
	}
	return fmt.Errorf("%s: %w (%s)", label, err, strings.TrimSpace(string(out)))
}

func parseTemplate(fs embed.FS, path, name string) *template.Template {
	b, err := fs.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return template.Must(template.New(name).Parse(string(b)))
}

func executeTemplate(t *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
