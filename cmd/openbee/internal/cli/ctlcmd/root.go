package ctlcmd

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/ctlclient"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

var ctlCfgPath string

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ctl",
		Short: i18n.M.Cmd.Ctl.Short,
	}
	cmd.PersistentFlags().StringVarP(&ctlCfgPath, "config", "c", "config.yaml", "path to config file")
	return cmd
}

// NewCommand constructs the wired-up ctl command with all subcommands registered.
func NewCommand() *cobra.Command {
	cmd := newRootCommand()
	cmd.AddCommand(
		newWorkerCommand(),
		newTaskCommand(),
		newConstraintCommand(),
		newSessionCommand(),
		newSystemCommand(),
		newMessageCommand(),
		newDepartmentCommand(),
	)
	return cmd
}

func ctlRun(toolName string, args any) error {
	c, err := ctlclient.NewClient(ctlCfgPath)
	if err != nil {
		return err
	}
	result, err := c.Call(toolName, args)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, result, "", "  "); err != nil {
		fmt.Println(string(result))
		return nil
	}
	fmt.Println(buf.String())
	return nil
}
