//go:build windows

package servicecmd

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"text/template"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

//go:embed templates/schtask.xml.tmpl
var windowsTemplatesFS embed.FS

const schtaskName = "OpenBee"

var (
	execLookPath = exec.LookPath
	runCommand   = defaultRunCommand
)

func defaultRunCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

type schtaskTemplateData struct {
	UserId     string
	ExePath    string
	ConfigPath string
	LogPath    string
}

func renderSchtaskXML(d schtaskTemplateData) (string, error) {
	b, err := windowsTemplatesFS.ReadFile("templates/schtask.xml.tmpl")
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("schtask").Parse(string(b))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
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

func (windowsManager) taskExists(ctx context.Context) bool {
	_, err := runCommand(ctx, "schtasks", "/Query", "/TN", schtaskName)
	return err == nil
}

func (m windowsManager) Install(ctx context.Context, opts InstallOptions) error {
	if m.taskExists(ctx) && !opts.Force {
		return errors.New(i18n.M.Output.Service.AlreadyInstalled)
	}
	uid, err := currentUserID()
	if err != nil {
		return err
	}
	xml, err := renderSchtaskXML(schtaskTemplateData{
		UserId:     uid,
		ExePath:    opts.ExePath,
		ConfigPath: opts.ConfigPath,
		LogPath:    opts.LogPath,
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
		return err
	}
	tmp.Close()

	args := []string{"/Create", "/XML", tmp.Name(), "/TN", schtaskName, "/F"}
	if out, err := runCommand(ctx, "schtasks", args...); err != nil {
		return fmt.Errorf("schtasks /Create: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	if !opts.AutoStart {
		return nil
	}
	if out, err := runCommand(ctx, "schtasks", "/Run", "/TN", schtaskName); err != nil {
		return fmt.Errorf("schtasks /Run: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (windowsManager) Uninstall(ctx context.Context) error {
	_, _ = runCommand(ctx, "schtasks", "/End", "/TN", schtaskName)
	if out, err := runCommand(ctx, "schtasks", "/Delete", "/TN", schtaskName, "/F"); err != nil {
		if strings.Contains(strings.ToLower(string(out)), "cannot find") {
			return nil
		}
		return fmt.Errorf("schtasks /Delete: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (windowsManager) Start(ctx context.Context) error {
	if out, err := runCommand(ctx, "schtasks", "/Run", "/TN", schtaskName); err != nil {
		return fmt.Errorf("schtasks /Run: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (windowsManager) Stop(ctx context.Context) error {
	if out, err := runCommand(ctx, "schtasks", "/End", "/TN", schtaskName); err != nil {
		return fmt.Errorf("schtasks /End: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m windowsManager) Status(ctx context.Context) (Status, error) {
	st := Status{}
	if !m.taskExists(ctx) {
		return st, nil
	}
	st.Installed = true
	out, err := runCommand(ctx, "schtasks", "/Query", "/TN", schtaskName, "/V", "/FO", "LIST")
	if err != nil {
		st.RunState = RunStateUnknown
		return st, nil
	}
	status := parseSchtasksField(string(out), "Status:")
	switch strings.TrimSpace(status) {
	case "Running":
		st.RunState = RunStateRunning
	case "Ready":
		st.RunState = RunStateStopped
	default:
		st.RunState = RunStateUnknown
	}
	if st.RunState == RunStateRunning {
		if pid := lookupOpenbeePID(ctx); pid > 0 {
			st.PID = pid
		}
	}
	return st, nil
}

func parseSchtasksField(s, key string) string {
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, key); i >= 0 {
			return strings.TrimSpace(line[i+len(key):])
		}
	}
	return ""
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
	var buf bytes.Buffer
	buf.Write([]byte{0xFF, 0xFE})
	for _, r := range s {
		if r > 0xFFFF {
			r -= 0x10000
			hi := uint16(0xD800 + (r >> 10))
			lo := uint16(0xDC00 + (r & 0x3FF))
			buf.WriteByte(byte(hi))
			buf.WriteByte(byte(hi >> 8))
			buf.WriteByte(byte(lo))
			buf.WriteByte(byte(lo >> 8))
		} else {
			buf.WriteByte(byte(r))
			buf.WriteByte(byte(r >> 8))
		}
	}
	return buf.Bytes()
}
