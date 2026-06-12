//go:build linux

package servicecmd

import (
	"context"
	"errors"
)

var errLinuxNotImplemented = errors.New("openbee service: Linux systemd support not yet implemented")

type linuxManager struct{}

// NewManager returns the Linux systemd --user service manager.
func NewManager() (Manager, error) { return linuxManager{}, nil }

func (linuxManager) Install(context.Context, InstallOptions) error { return errLinuxNotImplemented }
func (linuxManager) Uninstall(context.Context) error               { return errLinuxNotImplemented }
func (linuxManager) Start(context.Context) error                   { return errLinuxNotImplemented }
func (linuxManager) Stop(context.Context) error                    { return errLinuxNotImplemented }
func (linuxManager) Status(context.Context) (Status, error) {
	return Status{}, errLinuxNotImplemented
}
