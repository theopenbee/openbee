//go:build !darwin && !linux

package servicecmd

import (
	"context"
	"fmt"
	"runtime"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func unsupportedErr() error {
	return fmt.Errorf(i18n.M.Output.Service.Unsupported, runtime.GOOS)
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
