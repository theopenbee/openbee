package plugincmd

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func stateLabel(state string) string {
	if l := i18n.M.Output.Plugin.States[state]; l != "" {
		return l
	}
	return state
}

func displayVersion(v string) string {
	if v == "" {
		return "-"
	}
	return v
}

func (d *deps) newListCommand(client func() (*Client, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: i18n.M.Cmd.Plugin.Sub("list"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client()
			if err != nil {
				return err
			}
			list, err := c.List(cmd.Context())
			if err != nil {
				return err
			}
			m := i18n.M.Output.Plugin
			if len(list) == 0 {
				fmt.Fprintln(d.Out, m.ListEmpty)
				return nil
			}
			w := tabwriter.NewWriter(d.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, m.ListHeader)
			for _, p := range list {
				bot := p.BotName
				if bot == "" {
					bot = "-"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.ID, p.Name.Get(d.Lang), displayVersion(p.Version), stateLabel(p.State), bot)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			for _, p := range list {
				if p.Error != "" {
					fmt.Fprintf(d.Out, "%s: %s\n", p.ID, p.Error)
				}
			}
			return nil
		},
	}
}

func (d *deps) newEnableCommand(client func() (*Client, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "enable <id>",
		Short: i18n.M.Cmd.Plugin.Sub("enable"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client()
			if err != nil {
				return err
			}
			if err := c.Enable(cmd.Context(), args[0]); err != nil {
				return err
			}
			return waitRunning(cmd.Context(), c, args[0], d.Out, d.Interval, d.RunTimeout)
		},
	}
}

func (d *deps) newDisableCommand(client func() (*Client, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "disable <id>",
		Short: i18n.M.Cmd.Plugin.Sub("disable"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client()
			if err != nil {
				return err
			}
			if err := c.Disable(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(d.Out, i18n.M.Output.Plugin.Disabled+"\n", args[0])
			return nil
		},
	}
}

func (d *deps) newRestartCommand(client func() (*Client, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "restart <id>",
		Short: i18n.M.Cmd.Plugin.Sub("restart"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client()
			if err != nil {
				return err
			}
			if err := c.Restart(cmd.Context(), args[0]); err != nil {
				return err
			}
			return waitRunning(cmd.Context(), c, args[0], d.Out, d.Interval, d.RunTimeout)
		},
	}
}

func (d *deps) newRemoveCommand(client func() (*Client, error)) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: i18n.M.Cmd.Plugin.Sub("remove"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m := i18n.M.Output.Plugin
			id := args[0]
			c, err := client()
			if err != nil {
				return err
			}
			if !yes {
				if !d.Prompter.Interactive() {
					return errors.New(m.RemoveNeedsYes)
				}
				ok, err := d.Prompter.Confirm(fmt.Sprintf(m.RemoveConfirm, id), false)
				if err != nil || !ok {
					return err
				}
			}
			op, err := c.Uninstall(cmd.Context(), id)
			if err != nil {
				return err
			}
			op, err = waitOperation(cmd.Context(), c, op.ID, d.Interval, d.OpTimeout)
			if errors.Is(err, errOperationTimeout) {
				fmt.Fprintf(d.Out, m.OpTimeout+"\n", id)
				return nil
			}
			if err != nil {
				return err
			}
			if op.Status == "failed" {
				return errors.New(op.Error)
			}
			fmt.Fprintf(d.Out, m.Removed+"\n", id)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, i18n.M.Output.Plugin.YesFlag)
	return cmd
}

func friendlyError(err error, cfgPath string) error {
	m := i18n.M.Output.Plugin
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotRunning):
		url := ""
		if _, after, ok := strings.Cut(err.Error(), " at "); ok {
			url = after
		}
		return fmt.Errorf(m.NotRunning, url)
	case errors.Is(err, errAgentEnvironment):
		return errors.New(m.AgentEnvironment)
	case errors.Is(err, errNoTokenSecret):
		return fmt.Errorf(m.NoTokenSecret, cfgPath)
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
		return fmt.Errorf(m.Unauthorized, apiErr.URL, cfgPath)
	}
	return err
}
