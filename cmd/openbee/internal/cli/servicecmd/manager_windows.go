//go:build windows

package servicecmd

import (
	"context"
	"embed"
	"encoding/binary"
	"errors"
	"os"
	"os/user"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

//go:embed templates/schtask.xml.tmpl
var windowsTemplatesFS embed.FS

const schtaskName = "OpenBee"

var schtaskTmpl = parseTemplate(windowsTemplatesFS, "templates/schtask.xml.tmpl", "schtask")

type schtaskTemplateData struct {
	UserId     string
	ExePath    string
	ConfigPath string
	LogPath    string
	WorkingDir string
}

func renderSchtaskXML(d schtaskTemplateData) (string, error) {
	return executeTemplate(schtaskTmpl, d)
}

type windowsManager struct{}

func NewManager() (Manager, error) { return windowsManager{}, nil }

func currentUserID() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

func (windowsManager) Install(ctx context.Context, opts InstallOptions) error {
	username, err := currentUserID()
	if err != nil {
		return err
	}
	xml, err := renderSchtaskXML(schtaskTemplateData{
		UserId:     username,
		ExePath:    opts.ExePath,
		ConfigPath: opts.ConfigPath,
		LogPath:    opts.LogPath,
		WorkingDir: opts.WorkingDir,
	})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "openbee-schtask-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(encodeUTF16LE(xml)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	args := []string{"/Create", "/XML", tmp.Name(), "/TN", schtaskName}
	if opts.Force {
		args = append(args, "/F")
	}
	out, err := runCommand(ctx, "schtasks", args...)
	if err != nil {
		if !opts.Force && strings.Contains(strings.ToLower(string(out)), "already exists") {
			return errors.New(i18n.M.Output.Service.AlreadyInstalled)
		}
		return wrapRunErr("schtasks /Create", err, out)
	}
	if !opts.AutoStart {
		return nil
	}
	_, err = runOrWrap(ctx, "schtasks", "/Run", "/TN", schtaskName)
	return err
}

func (windowsManager) Uninstall(ctx context.Context) error {
	out, err := runCommand(ctx, "schtasks", "/Delete", "/TN", schtaskName, "/F")
	if err != nil {
		if strings.Contains(strings.ToLower(string(out)), "cannot find") {
			return nil
		}
		return wrapRunErr("schtasks /Delete", err, out)
	}
	return nil
}

func (windowsManager) Start(ctx context.Context) error {
	_, err := runOrWrap(ctx, "schtasks", "/Run", "/TN", schtaskName)
	return err
}

func (windowsManager) Stop(ctx context.Context) error {
	_, err := runOrWrap(ctx, "schtasks", "/End", "/TN", schtaskName)
	return err
}

// Status reports installation and run state without parsing locale-sensitive
// text. `schtasks /Query /V /FO LIST` would have worked on English Windows but
// localizes both the field name ("Status:" → "状态:") and the value
// ("Running" → "正在运行") on non-English systems, which historically pinned
// RunState to Unknown and made `service install` falsely report failure.
func (windowsManager) Status(ctx context.Context) (Status, error) {
	st := Status{}
	if _, err := runCommand(ctx, "schtasks", "/Query", "/TN", schtaskName); err != nil {
		return st, nil
	}
	st.Installed = true

	switch queryScheduledTaskState(ctx) {
	case "Running":
		st.RunState = RunStateRunning
	case "Ready", "Disabled":
		st.RunState = RunStateStopped
	default:
		// PowerShell unavailable or returned an unexpected value: infer running
		// state from a live openbee.exe process. We cannot distinguish "task
		// stopped" from "PowerShell broken" in this branch, so we only flip to
		// Running when we actually see a process; otherwise RunState stays
		// Unknown rather than misreporting Stopped.
		if pid := lookupOpenbeePID(ctx); pid > 0 {
			st.RunState = RunStateRunning
			st.PID = pid
		}
	}
	if st.RunState == RunStateRunning && st.PID == 0 {
		if pid := lookupOpenbeePID(ctx); pid > 0 {
			st.PID = pid
		}
	}
	return st, nil
}

// queryScheduledTaskState returns the Task Scheduler state for the OpenBee
// task — "Ready", "Running", "Disabled", or similar — via PowerShell's
// Get-ScheduledTask, whose enum output is in stable English regardless of the
// system display language. Returns "" when PowerShell is unavailable or the
// call fails so callers can fall back.
func queryScheduledTaskState(ctx context.Context) string {
	out, err := runCommand(ctx, "powershell",
		"-NoProfile", "-NonInteractive",
		"-Command", "(Get-ScheduledTask -TaskName '"+schtaskName+"' -ErrorAction Stop).State")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func lookupOpenbeePID(ctx context.Context) int {
	out, err := runCommand(ctx, "tasklist", "/FI", "IMAGENAME eq openbee.exe", "/FO", "CSV", "/NH")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(line, ",")
		if len(fields) >= 2 {
			pidStr := strings.Trim(fields[1], `" `)
			if pid, err := strconv.Atoi(pidStr); err == nil {
				return pid
			}
		}
	}
	return 0
}

func encodeUTF16LE(s string) []byte {
	units := utf16.Encode([]rune(s))
	buf := make([]byte, 2+2*len(units))
	buf[0], buf[1] = 0xFF, 0xFE
	for i, u := range units {
		binary.LittleEndian.PutUint16(buf[2+2*i:], u)
	}
	return buf
}
