package plugincmd

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func stateLabel(state string) string {
	if state == "not_configured" {
		return "not configured"
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
		Short: "List installed plugins and their state",
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
			if len(list) == 0 {
				fmt.Fprintln(d.Out, "No channel plugins installed.")
				return nil
			}
			w := tabwriter.NewWriter(d.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tVERSION\tSTATE\tBOT NAME")
			for _, p := range list {
				bot := p.BotName
				if bot == "" {
					bot = "-"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.ID, p.Name.Get(cliLang), displayVersion(p.Version), stateLabel(p.State), bot)
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
		Short: "Enable a plugin",
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
		Short: "Disable a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client()
			if err != nil {
				return err
			}
			if err := c.Disable(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(d.Out, "Disabled %s\n", args[0])
			return nil
		},
	}
}

func (d *deps) newRestartCommand(client func() (*Client, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "restart <id>",
		Short: "Restart a plugin",
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
		Short: "Remove a plugin with its settings and data",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			c, err := client()
			if err != nil {
				return err
			}
			if !yes {
				if !d.Prompter.Interactive() {
					return errors.New("Not running in a terminal; pass --yes to confirm the removal.")
				}
				ok, err := d.Prompter.Confirm(fmt.Sprintf("Remove %s? Its settings and data will be deleted.", id), false)
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
				fmt.Fprintf(d.Out, "%s is still being processed; check later with openbee plugin list\n", id)
				return nil
			}
			if err != nil {
				return err
			}
			if op.Status == "failed" {
				return errors.New(op.Error)
			}
			fmt.Fprintf(d.Out, "Removed plugin %s\n", id)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Remove without asking for confirmation")
	return cmd
}

func friendlyError(err error, cfgPath string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotRunning):
		url := ""
		if _, after, ok := strings.Cut(err.Error(), " at "); ok {
			url = after
		}
		return fmt.Errorf("openbee is not running at %s. Start it with: openbee server -d", url)
	case errors.Is(err, errAgentEnvironment):
		return errors.New("Plugin management is not available inside openbee agents.")
	case errors.Is(err, errNoTokenSecret):
		return fmt.Errorf("Set bee.rpc.token_secret in %s first, then restart openbee.", cfgPath)
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
		return fmt.Errorf("openbee at %s rejected the CLI credentials. Make sure it runs with %s and was restarted after bee.rpc.token_secret changed.", apiErr.URL, cfgPath)
	}
	return err
}
