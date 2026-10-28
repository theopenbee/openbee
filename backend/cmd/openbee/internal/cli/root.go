package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/backupcmd"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/configcmd"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/ctlcmd"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/daemoncmd"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/passwordcmd"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/plugincmd"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/servicecmd"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/upgradecmd"
)

// BuildInfo holds build-time metadata injected via -ldflags.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// NewRoot constructs the openbee root cobra command.
func NewRoot(info BuildInfo) *cobra.Command {
	root := &cobra.Command{
		Use:           "openbee",
		Short:         "OpenBee core service",
		Version:       info.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetVersionTemplate(fmt.Sprintf("openbee %s (commit: %s, built: %s)\n", info.Version, info.Commit, info.Date))
	exitCode := func(code int) error { return &ExitCodeError{Code: code} }
	root.AddCommand(
		daemoncmd.NewServerCommand(),
		daemoncmd.NewStopCommand(),
		daemoncmd.NewRestartCommand(),
		daemoncmd.NewStatusCommand(exitCode),
		backupcmd.NewBackupCommand(info.Version),
		backupcmd.NewRestoreCommand(info.Version),
		passwordcmd.NewResetCommand(),
	)
	root.AddCommand(ctlcmd.NewCommand())
	root.AddCommand(upgradecmd.NewCommand(info.Version))
	root.AddCommand(configcmd.NewCommand())
	root.AddCommand(plugincmd.NewCommand())
	root.AddCommand(servicecmd.NewCommand())
	return root
}
