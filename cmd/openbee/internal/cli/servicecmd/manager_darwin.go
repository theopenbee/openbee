//go:build darwin

package servicecmd

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"os/exec"
	"text/template"
)

//go:embed templates/launchd.plist.tmpl
var darwinTemplatesFS embed.FS

const launchdLabel = "com.theopenbee.openbee"

// Indirections for tests.
var (
	execLookPath = exec.LookPath
	runCommand   = defaultRunCommand
)

func defaultRunCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

type launchdTemplateData struct {
	ExePath    string
	ConfigPath string
	LogPath    string
	Home       string
	Path       string
}

func renderLaunchdPlist(d launchdTemplateData) (string, error) {
	b, err := darwinTemplatesFS.ReadFile("templates/launchd.plist.tmpl")
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("launchd").Parse(string(b))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

var errDarwinNotImplemented = errors.New("openbee service: macOS launchd support not yet implemented")

type darwinManager struct{}

func NewManager() (Manager, error) { return darwinManager{}, nil }

func (darwinManager) Install(context.Context, InstallOptions) error { return errDarwinNotImplemented }
func (darwinManager) Uninstall(context.Context) error               { return errDarwinNotImplemented }
func (darwinManager) Start(context.Context) error                   { return errDarwinNotImplemented }
func (darwinManager) Stop(context.Context) error                    { return errDarwinNotImplemented }
func (darwinManager) Status(context.Context) (Status, error) {
	return Status{}, errDarwinNotImplemented
}
