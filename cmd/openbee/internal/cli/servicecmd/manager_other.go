//go:build !darwin && !linux

package servicecmd

import (
	"context"
	"fmt"
	"runtime"
)

func unsupportedErr() error {
	return fmt.Errorf("openbee service is not supported on %s", runtime.GOOS)
}

type unsupportedManager struct{}

func NewManager() (Manager, error) { return unsupportedManager{}, nil }

func (unsupportedManager) Install(context.Context, InstallOptions) error { return unsupportedErr() }
func (unsupportedManager) Uninstall(context.Context) error               { return unsupportedErr() }
func (unsupportedManager) Start(context.Context) error                   { return unsupportedErr() }
func (unsupportedManager) Stop(context.Context) error                    { return unsupportedErr() }
func (unsupportedManager) Status(context.Context) (Status, error) {
	return Status{}, unsupportedErr()
}
