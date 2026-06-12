//go:build darwin

package servicecmd

import (
	"context"
	"errors"
)

var errDarwinNotImplemented = errors.New("openbee service: macOS launchd support not yet implemented")

type darwinManager struct{}

// NewManager returns the macOS launchd-based service manager.
func NewManager() (Manager, error) { return darwinManager{}, nil }

func (darwinManager) Install(context.Context, InstallOptions) error { return errDarwinNotImplemented }
func (darwinManager) Uninstall(context.Context) error               { return errDarwinNotImplemented }
func (darwinManager) Start(context.Context) error                   { return errDarwinNotImplemented }
func (darwinManager) Stop(context.Context) error                    { return errDarwinNotImplemented }
func (darwinManager) Status(context.Context) (Status, error) {
	return Status{}, errDarwinNotImplemented
}
