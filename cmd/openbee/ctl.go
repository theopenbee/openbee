package main

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/ctlclient"
	"github.com/theopenbee/openbee/internal/i18n"
)

var ctlCfgPath string

var ctlCmd = &cobra.Command{
	Use:   "ctl",
	Short: "", // set by applyTranslations
}

func init() {
	ctlCmd.PersistentFlags().StringVarP(&ctlCfgPath, "config", "c", "config.yaml", "path to config file")
	rootCmd.AddCommand(ctlCmd)
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

// applyCtlTranslations sets Short for all ctl commands from i18n.M.
func applyCtlTranslations() {
	m := i18n.M
	ctlCmd.Short = m.Cmd.Ctl.Short
	ctlWorkerCmd.Short = m.Cmd.CtlWorker.Short
	ctlTaskCmd.Short = m.Cmd.CtlTask.Short
	ctlMemoryCmd.Short = m.Cmd.CtlMemory.Short
	ctlSessionCmd.Short = m.Cmd.CtlSession.Short
	ctlSystemCmd.Short = m.Cmd.CtlSystem.Short
	ctlMessageCmd.Short = m.Cmd.CtlMessage.Short
}
