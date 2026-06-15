package servicecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

// verifyRunningTimeout bounds how long reportRunStateAfterStart waits for the
// service to enter the running state. Exposed as a var so tests can shrink it.
var (
	verifyRunningTimeout = 1500 * time.Millisecond
	verifyRunningPoll    = 150 * time.Millisecond
)

// reportRunStateAfterStart polls the manager until the service is running or
// the deadline expires. On success it prints the started message; on failure
// it prints diagnostics (last exit code/reason and a tail of the daemon log)
// and returns an error so the CLI exits non-zero.
func reportRunStateAfterStart(ctx context.Context, mgr Manager, out io.Writer) error {
	st, err := waitRunning(ctx, mgr, verifyRunningTimeout)
	if err != nil {
		return err
	}
	if st.RunState == RunStateRunning {
		fmt.Fprintln(out, i18n.M.Output.Service.Started)
		if st.PID > 0 {
			fmt.Fprintf(out, i18n.M.Output.Service.StatusPIDUptime+"\n", st.PID, utils.FormatUptime(st.UptimeSecs))
		}
		return nil
	}
	printStartFailureDetails(out, st)
	return errors.New(i18n.M.Output.Service.StartFailedSeeStatus)
}

func waitRunning(ctx context.Context, mgr Manager, timeout time.Duration) (Status, error) {
	deadline := time.Now().Add(timeout)
	for {
		st, err := mgr.Status(ctx)
		if err != nil {
			return st, err
		}
		if st.RunState == RunStateRunning {
			return st, nil
		}
		if !time.Now().Before(deadline) {
			return st, nil
		}
		select {
		case <-ctx.Done():
			return st, ctx.Err()
		case <-time.After(verifyRunningPoll):
		}
	}
}

func printStartFailureDetails(out io.Writer, st Status) {
	if st.LastExitCode != "" {
		fmt.Fprintf(out, i18n.M.Output.Service.StatusLastExitCode+"\n", st.LastExitCode)
	}
	if st.LastExitReason != "" {
		fmt.Fprintf(out, i18n.M.Output.Service.StatusLastExitReason+"\n", st.LastExitReason)
	}
	logPath, err := config.DaemonLogFile()
	if err != nil {
		return
	}
	fmt.Fprintf(out, i18n.M.Output.Service.StatusLogPath+"\n", logPath)
	tail, err := tailFile(logPath, 64*1024, 20)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(out, i18n.M.Output.Service.StatusLogReadFailed+"\n", err)
		}
		return
	}
	if tail == "" {
		return
	}
	fmt.Fprintln(out, i18n.M.Output.Service.StatusLogTailHeader)
	fmt.Fprintln(out, tail)
}

func tailFile(path string, maxBytes int64, maxLines int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	var truncated bool
	if size := info.Size(); size > maxBytes {
		if _, err := f.Seek(size-maxBytes, io.SeekStart); err != nil {
			return "", err
		}
		truncated = true
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	text := string(data)
	if truncated {
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		}
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n"), nil
}
