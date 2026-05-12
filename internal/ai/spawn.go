package ai

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// SubprocessSpec describes a subprocess to launch via SpawnSubprocess. Stdin
// is supplied via Stdin (when non-empty); stdout and stderr both go to LogPath
// unless StdoutOverride/StderrOverride are non-nil.
type SubprocessSpec struct {
	Binary         string
	Args           []string
	WorkDir        string
	LogPath        string
	Env            []string
	Stdin          string
	StdoutOverride io.Writer
	StderrOverride io.Writer
	// PostWait, if non-nil, runs after cmd.Wait() returns. A non-nil return
	// becomes the terminal Output event; a nil return falls back to the
	// default mapping (waitErr → OutputError, nil → OutputDone).
	PostWait func(waitErr error, logPath string) *Output
}

// SpawnSubprocess starts a subprocess described by spec, redirects output to
// spec.LogPath, and returns a Process plus a 1-buffered channel that receives
// exactly one Output (Done or Error) when the process exits.
func SpawnSubprocess(ctx context.Context, spec SubprocessSpec) (Process, <-chan Output, error) {
	logFile, err := os.OpenFile(spec.LogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.CommandContext(ctx, spec.Binary, spec.Args...)
	cmd.Dir = spec.WorkDir
	if spec.Stdin != "" {
		cmd.Stdin = strings.NewReader(spec.Stdin)
	}
	if spec.StdoutOverride != nil {
		cmd.Stdout = spec.StdoutOverride
	} else {
		cmd.Stdout = logFile
	}
	if spec.StderrOverride != nil {
		cmd.Stderr = spec.StderrOverride
	} else {
		cmd.Stderr = logFile
	}
	cmd.Env = spec.Env
	ConfigureCmd(cmd)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("start subprocess: %w", err)
	}

	proc := NewCmdProcess(cmd)
	ch := make(chan Output, 1)

	go func() {
		defer close(ch)
		defer logFile.Close()
		waitErr := cmd.Wait()
		if spec.PostWait != nil {
			if ev := spec.PostWait(waitErr, spec.LogPath); ev != nil {
				ch <- *ev
				return
			}
		}
		if waitErr != nil {
			ch <- Output{Type: OutputError, Content: waitErr.Error()}
		} else {
			ch <- Output{Type: OutputDone}
		}
	}()

	return proc, ch, nil
}
