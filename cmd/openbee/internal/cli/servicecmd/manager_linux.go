//go:build linux

package servicecmd

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"text/template"
)

//go:embed templates/systemd.service.tmpl
var linuxTemplatesFS embed.FS

const systemdUnitName = "openbee.service"

type systemdTemplateData struct {
	ExePath    string
	ConfigPath string
	LogPath    string
	Home       string
}

func renderSystemdUnit(d systemdTemplateData) (string, error) {
	b, err := linuxTemplatesFS.ReadFile("templates/systemd.service.tmpl")
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("systemd").Parse(string(b))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

var errLinuxNotImplemented = errors.New("openbee service: systemd support not yet wired")

type linuxManager struct{}

func NewManager() (Manager, error) { return linuxManager{}, nil }

func (linuxManager) Install(context.Context, InstallOptions) error { return errLinuxNotImplemented }
func (linuxManager) Uninstall(context.Context) error               { return errLinuxNotImplemented }
func (linuxManager) Start(context.Context) error                   { return errLinuxNotImplemented }
func (linuxManager) Stop(context.Context) error                    { return errLinuxNotImplemented }
func (linuxManager) Status(context.Context) (Status, error)        { return Status{}, errLinuxNotImplemented }
