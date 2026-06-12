//go:build !darwin && !linux && !windows

package servicecmd

import (
	"context"
	"errors"
	"runtime"
)

var errUnsupportedOS = errors.New("openbee service is not supported on " + runtime.GOOS)

type unsupportedManager struct{}

func NewManager() (Manager, error) { return unsupportedManager{}, nil }

func (unsupportedManager) Install(context.Context, InstallOptions) error { return errUnsupportedOS }
func (unsupportedManager) Uninstall(context.Context) error               { return errUnsupportedOS }
func (unsupportedManager) Start(context.Context) error                   { return errUnsupportedOS }
func (unsupportedManager) Stop(context.Context) error                    { return errUnsupportedOS }
func (unsupportedManager) Status(context.Context) (Status, error) {
	return Status{}, errUnsupportedOS
}
