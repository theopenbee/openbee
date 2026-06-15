//go:build linux

package servicecmd

import (
	"context"
	"embed"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

//go:embed templates/systemd.service.tmpl
var linuxTemplatesFS embed.FS

const systemdUnitName = "openbee.service"

var systemdTmpl = parseTemplate(linuxTemplatesFS, "templates/systemd.service.tmpl", "systemd")

type systemdTemplateData struct {
	ExePath    string
	ConfigPath string
	LogPath    string
	WorkingDir string
	Home       string
	EnvPath    string
}

func renderSystemdUnit(d systemdTemplateData) (string, error) {
	return executeTemplate(systemdTmpl, d)
}

type linuxManager struct{}

func NewManager() (Manager, error) {
	if _, err := execLookPath("systemctl"); err != nil {
		return nil, errors.New(i18n.M.Output.Service.SystemdUnavail)
	}
	return linuxManager{}, nil
}

func (linuxManager) unitPath() (string, error) {
	cfgHome, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgHome, "systemd", "user", systemdUnitName), nil
}

func (m linuxManager) Install(ctx context.Context, opts InstallOptions) error {
	up, err := m.unitPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(up); err == nil && !opts.Force {
		return errors.New(i18n.M.Output.Service.AlreadyInstalled)
	}
	if err := os.MkdirAll(filepath.Dir(up), 0o755); err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	unit, err := renderSystemdUnit(systemdTemplateData{
		ExePath:    opts.ExePath,
		ConfigPath: opts.ConfigPath,
		LogPath:    opts.LogPath,
		WorkingDir: opts.WorkingDir,
		Home:       home,
		EnvPath:    opts.EnvPath,
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(up, []byte(unit), 0o644); err != nil {
		return err
	}
	if _, err := runOrWrap(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	enableArgs := []string{"--user", "enable", systemdUnitName}
	if opts.AutoStart {
		enableArgs = []string{"--user", "enable", "--now", systemdUnitName}
	}
	if _, err := runOrWrap(ctx, "systemctl", enableArgs...); err != nil {
		return err
	}
	return nil
}

func (m linuxManager) Uninstall(ctx context.Context) error {
	up, err := m.unitPath()
	if err != nil {
		return err
	}
	_, _ = runCommand(ctx, "systemctl", "--user", "disable", "--now", systemdUnitName)
	if err := os.Remove(up); err != nil && !os.IsNotExist(err) {
		return err
	}
	_, _ = runCommand(ctx, "systemctl", "--user", "daemon-reload")
	return nil
}

func (linuxManager) Start(ctx context.Context) error {
	_, err := runOrWrap(ctx, "systemctl", "--user", "start", systemdUnitName)
	return err
}

func (linuxManager) Stop(ctx context.Context) error {
	_, err := runOrWrap(ctx, "systemctl", "--user", "stop", systemdUnitName)
	return err
}

func (m linuxManager) Status(ctx context.Context) (Status, error) {
	up, err := m.unitPath()
	if err != nil {
		return Status{}, err
	}
	st := Status{}
	if _, err := os.Stat(up); err == nil {
		st.Installed = true
	}
	out, err := runCommand(ctx, "systemctl", "--user", "show", "-p", "ActiveState,SubState,MainPID,ExecMainStartTimestamp", systemdUnitName)
	if err != nil {
		if !st.Installed {
			return st, nil
		}
		st.RunState = RunStateStopped
		return st, nil
	}
	props := parseSystemctlShow(string(out))
	switch props["ActiveState"] {
	case "active":
		st.RunState = RunStateRunning
	case "failed":
		st.RunState = RunStateFailed
	default:
		st.RunState = RunStateStopped
	}
	if pid, err := strconv.Atoi(props["MainPID"]); err == nil && pid > 0 {
		st.PID = pid
	}
	return st, nil
}

func parseSystemctlShow(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, "="); i > 0 {
			out[line[:i]] = line[i+1:]
		}
	}
	return out
}
