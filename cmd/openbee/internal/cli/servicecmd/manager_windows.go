//go:build windows

package servicecmd

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"text/template"
)

//go:embed templates/schtask.xml.tmpl
var windowsTemplatesFS embed.FS

const schtaskName = "OpenBee"

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

var errWindowsNotImplemented = errors.New("openbee service: Task Scheduler support not yet wired")

type windowsManager struct{}

func NewManager() (Manager, error) { return windowsManager{}, nil }

func (windowsManager) Install(context.Context, InstallOptions) error { return errWindowsNotImplemented }
func (windowsManager) Uninstall(context.Context) error               { return errWindowsNotImplemented }
func (windowsManager) Start(context.Context) error                   { return errWindowsNotImplemented }
func (windowsManager) Stop(context.Context) error                    { return errWindowsNotImplemented }
func (windowsManager) Status(context.Context) (Status, error)        { return Status{}, errWindowsNotImplemented }
