//go:build windows

package servicecmd

import (
	"context"
	"errors"
)

var errWindowsNotImplemented = errors.New("openbee service: Windows Task Scheduler support not yet implemented")

type windowsManager struct{}

// NewManager returns the Windows Task Scheduler service manager.
func NewManager() (Manager, error) { return windowsManager{}, nil }

func (windowsManager) Install(context.Context, InstallOptions) error {
	return errWindowsNotImplemented
}
func (windowsManager) Uninstall(context.Context) error { return errWindowsNotImplemented }
func (windowsManager) Start(context.Context) error     { return errWindowsNotImplemented }
func (windowsManager) Stop(context.Context) error      { return errWindowsNotImplemented }
func (windowsManager) Status(context.Context) (Status, error) {
	return Status{}, errWindowsNotImplemented
}
