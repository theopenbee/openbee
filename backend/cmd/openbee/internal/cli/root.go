package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/backupcmd"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/configcmd"
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
	root.AddCommand(upgradecmd.NewCommand(info.Version))
	root.AddCommand(configcmd.NewCommand())
	root.AddCommand(plugincmd.NewCommand())
	root.AddCommand(servicecmd.NewCommand())
	root.AddCommand(newRemovedCtlCommand())
	return root
}

const ctlRemoved = "openbee ctl has been removed. Agents reach openbee through the openbee MCP server: " +
	"call its send_message tool to talk to the user and its execute tool for everything else. " +
	"Load the openbee-bee or openbee-worker skill again for details."

func newRemovedCtlCommand() *cobra.Command {
	return &cobra.Command{
		Use:                "ctl",
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(*cobra.Command, []string) error {
			return errors.New(ctlRemoved)
		},
	}
}
