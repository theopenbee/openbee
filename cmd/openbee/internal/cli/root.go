package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/backupcmd"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/ctlcmd"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/daemoncmd"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/upgradecmd"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

// BuildInfo holds build-time metadata injected via -ldflags.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// NewRoot constructs the openbee root cobra command. It assumes i18n has already
// been loaded by the caller so that translated short/long fields are available.
func NewRoot(info BuildInfo) *cobra.Command {
	root := &cobra.Command{
		Use:           "openbee",
		Short:         i18n.M.Cmd.Root.Short,
		Version:       info.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetVersionTemplate(fmt.Sprintf("openbee %s (commit: %s, built: %s)\n", info.Version, info.Commit, info.Date))
	exitCode := func(code int) error { return &ExitCodeError{Code: code} }
	root.AddCommand(
		daemoncmd.NewServerCommand(),
		daemoncmd.NewStopCommand(exitCode),
		daemoncmd.NewRestartCommand(exitCode),
		daemoncmd.NewStatusCommand(exitCode),
		backupcmd.NewBackupCommand(info.Version),
		backupcmd.NewRestoreCommand(info.Version),
	)
	root.AddCommand(ctlcmd.NewCommand())
	root.AddCommand(upgradecmd.NewCommand(info.Version))
	return root
}
