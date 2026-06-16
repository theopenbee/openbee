//go:build darwin

package servicecmd

import (
	"context"
	"embed"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

//go:embed templates/launchd.plist.tmpl
var darwinTemplatesFS embed.FS

const launchdLabel = "com.theopenbee.openbee"

var launchdTmpl = parseTemplate(darwinTemplatesFS, "templates/launchd.plist.tmpl", "launchd")

type launchdTemplateData struct {
	ExePath    string
	ConfigPath string
	LogPath    string
	WorkingDir string
	Home       string
	EnvPath    string
}

func renderLaunchdPlist(d launchdTemplateData) (string, error) {
	return executeTemplate(launchdTmpl, d)
}

type darwinManager struct{}

func NewManager() (Manager, error) { return darwinManager{}, nil }

func (darwinManager) plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist"), nil
}

func (m darwinManager) Install(ctx context.Context, opts InstallOptions) error {
	pp, err := m.plistPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(pp); err == nil && !opts.Force {
		return errors.New(i18n.M.Output.Service.AlreadyInstalled)
	}
	if err := os.MkdirAll(filepath.Dir(pp), 0o755); err != nil {
		return err
	}
	plist, err := renderLaunchdPlist(launchdTemplateData{
		ExePath:    opts.ExePath,
		ConfigPath: opts.ConfigPath,
		LogPath:    opts.LogPath,
		WorkingDir: opts.WorkingDir,
		Home:       opts.Home,
		EnvPath:    opts.EnvPath,
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(pp, []byte(plist), 0o644); err != nil {
		return err
	}
	if _, err := runOrWrap(ctx, "launchctl", "bootstrap", guiTarget(), pp); err != nil {
		return err
	}
	if !opts.AutoStart {
		return nil
	}
	_, err = runOrWrap(ctx, "launchctl", "kickstart", launchdTarget())
	return err
}

func (m darwinManager) Uninstall(ctx context.Context) error {
	pp, err := m.plistPath()
	if err != nil {
		return err
	}
	_, _ = runCommand(ctx, "launchctl", "bootout", launchdTarget())
	if err := os.Remove(pp); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (m darwinManager) Start(ctx context.Context) error {
	if m.isLoaded(ctx) {
		_, err := runOrWrap(ctx, "launchctl", "kickstart", launchdTarget())
		return err
	}
	pp, err := m.plistPath()
	if err != nil {
		return err
	}
	_, err = runOrWrap(ctx, "launchctl", "bootstrap", guiTarget(), pp)
	return err
}

// Stop unloads the launchd job rather than just killing the process; the plist
// has KeepAlive=true, so a plain SIGTERM would be respawned immediately.
func (darwinManager) Stop(ctx context.Context) error {
	_, err := runOrWrap(ctx, "launchctl", "bootout", launchdTarget())
	return err
}

func (darwinManager) isLoaded(ctx context.Context) bool {
	_, err := runCommand(ctx, "launchctl", "print", launchdTarget())
	return err == nil
}

func (m darwinManager) Status(ctx context.Context) (Status, error) {
	pp, err := m.plistPath()
	if err != nil {
		return Status{}, err
	}
	st := Status{}
	if _, err := os.Stat(pp); err != nil {
		return st, nil
	}
	st.Installed = true
	out, err := runCommand(ctx, "launchctl", "print", launchdTarget())
	if err != nil {
		st.RunState = RunStateStopped
		return st, nil
	}
	props := parseLaunchctlPrint(string(out))
	if strings.HasPrefix(strings.ToLower(props["state"]), "running") {
		st.RunState = RunStateRunning
	} else {
		st.RunState = RunStateStopped
	}
	if pid, err := strconv.Atoi(props["pid"]); err == nil && pid > 0 {
		st.PID = pid
	}
	st.LastExitCode = props["last exit code"]
	st.LastExitReason = props["last exit reason"]
	return st, nil
}

// parseLaunchctlPrint scans `launchctl print` output once, splitting each
// `key = value` line into the result map. Keys may contain spaces
// (`last exit reason`), so we split on the first `=` only.
func parseLaunchctlPrint(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		i := strings.Index(line, "=")
		if i <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		out[key] = val
	}
	return out
}

func guiTarget() string {
	u, err := user.Current()
	if err != nil || u.Uid == "" {
		return "gui/0"
	}
	return "gui/" + u.Uid
}

func launchdTarget() string { return guiTarget() + "/" + launchdLabel }
