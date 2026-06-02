package ctlcmd

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/ctlclient"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

// Runner invokes a ctl tool against the server identified by the persistent --config flag.
type Runner func(toolName string, args any) error

// NewCommand constructs the wired-up ctl command with all subcommands registered.
func NewCommand() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "ctl",
		Short: i18n.M.Cmd.Ctl.Short,
	}
	cmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", "config.yaml", i18n.M.Flag.ConfigPath)

	run := func(toolName string, args any) error {
		return doCtlRun(cfgPath, toolName, args)
	}

	cmd.AddCommand(
		newWorkerCommand(run),
		newTaskCommand(run),
		newConstraintCommand(run),
		newSessionCommand(run),
		newSystemCommand(run),
		newMessageCommand(run),
		newDepartmentCommand(run),
	)
	return cmd
}

// doCtlRun calls the named tool, pretty-prints JSON output, and falls back to
// raw output if the response cannot be re-indented.
func doCtlRun(cfgPath, toolName string, args any) error {
	c, err := ctlclient.NewClient(cfgPath)
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
