//go:build darwin

package servicecmd

import (
	"context"
	"embed"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
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
	Home       string
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
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	plist, err := renderLaunchdPlist(launchdTemplateData{
		ExePath:    opts.ExePath,
		ConfigPath: opts.ConfigPath,
		LogPath:    opts.LogPath,
		Home:       home,
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
	if _, err := runOrWrap(ctx, "launchctl", "kickstart", guiTarget()+"/"+launchdLabel); err != nil {
		return err
	}
	return nil
}

func (m darwinManager) Uninstall(ctx context.Context) error {
	pp, err := m.plistPath()
	if err != nil {
		return err
	}
	_, _ = runCommand(ctx, "launchctl", "bootout", guiTarget()+"/"+launchdLabel)
	if err := os.Remove(pp); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (darwinManager) Start(ctx context.Context) error {
	_, err := runOrWrap(ctx, "launchctl", "kickstart", guiTarget()+"/"+launchdLabel)
	return err
}

func (darwinManager) Stop(ctx context.Context) error {
	_, err := runOrWrap(ctx, "launchctl", "kill", "SIGTERM", guiTarget()+"/"+launchdLabel)
	return err
}

var (
	launchdStateRe          = regexp.MustCompile(`state\s*=\s*(\w+)`)
	launchdPIDRe            = regexp.MustCompile(`pid\s*=\s*(\d+)`)
	launchdLastExitCodeRe   = regexp.MustCompile(`(?m)^\s*last exit code\s*=\s*(.+?)\s*$`)
	launchdLastExitReasonRe = regexp.MustCompile(`(?m)^\s*last exit reason\s*=\s*(.+?)\s*$`)
)

func (m darwinManager) Status(ctx context.Context) (Status, error) {
	pp, err := m.plistPath()
	if err != nil {
		return Status{}, err
	}
	st := Status{}
	if _, err := os.Stat(pp); err == nil {
		st.Installed = true
	}
	out, err := runCommand(ctx, "launchctl", "print", guiTarget()+"/"+launchdLabel)
	if err != nil {
		if !st.Installed {
			return st, nil
		}
		st.RunState = RunStateStopped
		return st, nil
	}
	text := string(out)
	if m := launchdStateRe.FindStringSubmatch(text); len(m) == 2 {
		if strings.Contains(strings.ToLower(m[1]), "running") {
			st.RunState = RunStateRunning
		} else {
			st.RunState = RunStateStopped
		}
	}
	if m := launchdPIDRe.FindStringSubmatch(text); len(m) == 2 {
		if pid, err := strconv.Atoi(m[1]); err == nil {
			st.PID = pid
			st.UptimeSecs = readUptime(ctx, pid)
		}
	}
	if m := launchdLastExitCodeRe.FindStringSubmatch(text); len(m) == 2 {
		st.LastExitCode = m[1]
	}
	if m := launchdLastExitReasonRe.FindStringSubmatch(text); len(m) == 2 {
		st.LastExitReason = m[1]
	}
	return st, nil
}

func guiTarget() string {
	u, err := user.Current()
	if err != nil || u.Uid == "" {
		return "gui/0"
	}
	return "gui/" + u.Uid
}
