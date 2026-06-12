//go:build darwin

package servicecmd

import (
	"context"
	"embed"
	"errors"
	"fmt"
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
	Path       string
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
	home, _ := os.UserHomeDir()
	plist, err := renderLaunchdPlist(launchdTemplateData{
		ExePath:    opts.ExePath,
		ConfigPath: opts.ConfigPath,
		LogPath:    opts.LogPath,
		Home:       home,
		Path:       os.Getenv("PATH"),
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(pp, []byte(plist), 0o644); err != nil {
		return err
	}
	if out, err := runCommand(ctx, "launchctl", "bootstrap", guiTarget(), pp); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	if !opts.AutoStart {
		return nil
	}
	if out, err := runCommand(ctx, "launchctl", "kickstart", guiTarget()+"/"+launchdLabel); err != nil {
		return fmt.Errorf("launchctl kickstart: %w (%s)", err, strings.TrimSpace(string(out)))
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
	if out, err := runCommand(ctx, "launchctl", "kickstart", guiTarget()+"/"+launchdLabel); err != nil {
		return fmt.Errorf("launchctl kickstart: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (darwinManager) Stop(ctx context.Context) error {
	if out, err := runCommand(ctx, "launchctl", "kill", "SIGTERM", guiTarget()+"/"+launchdLabel); err != nil {
		return fmt.Errorf("launchctl kill: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

var (
	launchdStateRe = regexp.MustCompile(`state\s*=\s*(\w+)`)
	launchdPIDRe   = regexp.MustCompile(`pid\s*=\s*(\d+)`)
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
			st.UptimeSecs = readUptime(pid)
		}
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
