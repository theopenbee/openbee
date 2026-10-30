package plugincmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/plugin"
)

func splitList(s string) []string {
	out := []string{}
	for _, part := range strings.Split(s, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func parseSets(fields []plugin.ConfigField, sets []string) (map[string]any, error) {
	types := make(map[string]plugin.FieldType, len(fields))
	for _, f := range fields {
		types[f.Key] = f.Type
	}
	out := make(map[string]any, len(sets))
	for _, set := range sets {
		key, value, ok := strings.Cut(set, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --set %q: expected key=value", set)
		}
		switch types[key] {
		case plugin.FieldBool:
			b, err := strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf("--set %s: %v", key, err)
			}
			out[key] = b
		case plugin.FieldInt:
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("--set %s: %v", key, err)
			}
			out[key] = n
		case plugin.FieldList:
			out[key] = splitList(value)
		default:
			out[key] = value
		}
	}
	return out, nil
}

func hasValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	}
	return true
}

func missingRequired(d Detail) bool {
	for _, f := range d.Fields {
		if f.Required && !hasValue(d.Config[f.Key]) {
			return true
		}
	}
	return false
}

func textDefault(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(x))
		for _, p := range x {
			parts = append(parts, fmt.Sprint(p))
		}
		return strings.Join(parts, ", ")
	}
	return fmt.Sprint(v)
}

func fieldLabel(f plugin.ConfigField) string {
	if l := f.Label.Get(cliLang); l != "" {
		return l
	}
	return f.Key
}

func collectSettings(d Detail, p Prompter, runSetup func(path string) (map[string]any, error), out io.Writer) (*string, map[string]any, error) {
	cfg := map[string]any{}
	if d.HasSetup {
		label := d.SetupLabel.Get(cliLang)
		if label == "" {
			label = "setup"
		}
		run, err := p.Confirm(fmt.Sprintf("Run %s now?", label), missingRequired(d))
		if err != nil {
			return nil, nil, err
		}
		if run {
			values, err := runSetup(d.Path)
			if err != nil {
				fmt.Fprintf(out, "%s failed: %v. Enter the settings manually.\n", label, err)
			}
			for k, v := range values {
				cfg[k] = v
			}
		}
	}
	for _, f := range d.Fields {
		if _, done := cfg[f.Key]; done {
			continue
		}
		if err := askField(f, d.Config, p, cfg, out); err != nil {
			return nil, nil, err
		}
	}
	if d.isEngine() {
		return nil, cfg, nil
	}
	fmt.Fprintln(out, "  The name users @mention in group chats; leave empty for none.")
	msg := "Bot name"
	if d.BotName != "" {
		msg = fmt.Sprintf("%s (enter - to clear)", msg)
	}
	name, err := p.Input(msg, "", d.BotName, nil)
	if err != nil {
		return nil, nil, err
	}
	if name = strings.TrimSpace(name); name == clearValue {
		name = ""
	}
	if name == d.BotName {
		return nil, cfg, nil
	}
	return &name, cfg, nil
}

const clearValue = "-"

func askField(f plugin.ConfigField, config map[string]any, p Prompter, cfg map[string]any, out io.Writer) error {
	current, present := config[f.Key]
	hasCur := present && hasValue(current)
	label := fieldLabel(f)
	if help := f.Help.Get(cliLang); help != "" {
		fmt.Fprintf(out, "  %s\n", help)
	}
	withDefault := func(msg string) string {
		if !present && f.Default != nil {
			return fmt.Sprintf("%s (default %s)", msg, textDefault(f.Default))
		}
		return msg
	}
	withClear := func(msg, def string) string {
		if def != "" {
			return fmt.Sprintf("%s (enter - to clear)", msg)
		}
		return msg
	}
	switch f.Type {
	case plugin.FieldSecret:
		msg := label
		if hasCur {
			msg = fmt.Sprintf("%s (press Enter to keep %s)", label, current)
		}
		for {
			v, err := p.Password(msg, "")
			if err != nil {
				return err
			}
			if v != "" {
				cfg[f.Key] = v
				return nil
			}
			if hasCur || !f.Required {
				return nil
			}
			fmt.Fprintf(out, "%s is required\n", label)
		}
	case plugin.FieldBool:
		def, _ := current.(bool)
		if !hasCur {
			def, _ = f.Default.(bool)
		}
		v, err := p.Confirm(label, def)
		if err != nil {
			return err
		}
		cfg[f.Key] = v
	case plugin.FieldInt:
		def := ""
		if hasCur {
			def = textDefault(current)
		}
		v, err := p.Input(withClear(withDefault(label), def), "", def, func(s string) error {
			s = strings.TrimSpace(s)
			if s == "" || s == clearValue {
				return nil
			}
			if _, err := strconv.Atoi(s); err != nil {
				return errors.New("Enter a whole number")
			}
			return nil
		})
		if err != nil {
			return err
		}
		switch v = strings.TrimSpace(v); v {
		case clearValue:
			cfg[f.Key] = nil
		case "":
		default:
			n, _ := strconv.Atoi(v)
			cfg[f.Key] = n
		}
	case plugin.FieldList:
		def := textDefault(current)
		v, err := p.Input(withClear(fmt.Sprintf("%s (comma-separated)", label), def), "", def, nil)
		if err != nil {
			return err
		}
		switch v = strings.TrimSpace(v); v {
		case clearValue:
			cfg[f.Key] = []string{}
		case "":
		default:
			cfg[f.Key] = splitList(v)
		}
	default:
		def := ""
		switch {
		case hasCur:
			def = textDefault(current)
		case f.RandomDefault && !present:
			def = randomHex(8)
		}
		required := f.Required && !hasCur
		v, err := p.Input(withClear(withDefault(label), def), "", def, func(s string) error {
			if required && (strings.TrimSpace(s) == "" || s == clearValue) {
				return fmt.Errorf("%s is required", label)
			}
			return nil
		})
		if err != nil {
			return err
		}
		switch {
		case v == clearValue:
			cfg[f.Key] = ""
		case v != "":
			cfg[f.Key] = v
		case present:
			cfg[f.Key] = ""
		}
	}
	return nil
}

func waitRunning(ctx context.Context, c *Client, id string, out io.Writer, interval, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		d, err := c.Get(ctx, id)
		if err != nil {
			return err
		}
		switch {
		case d.State == "running":
			fmt.Fprintf(out, "%s is running\n", id)
			return nil
		case d.State == "failed", d.State == "retrying" && d.Error != "":
			return fmt.Errorf("%s failed to start: %s", id, d.Error)
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(out, "%s is still starting; check later with openbee plugin list\n", id)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func (d *deps) newConfigureCommand(client func() (*Client, error)) *cobra.Command {
	var sets []string
	var botName string
	var enable bool
	cmd := &cobra.Command{
		Use:   "configure <id>",
		Short: "Configure a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			c, err := client()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			detail, err := c.Get(ctx, id)
			if err != nil {
				return err
			}
			if detail.isEngine() && cmd.Flags().Changed("bot-name") {
				return fmt.Errorf("%s is an engine and has no bot name; remove --bot-name", id)
			}
			var namePtr *string
			var cfg map[string]any
			if len(sets) > 0 || cmd.Flags().Changed("bot-name") {
				if cfg, err = parseSets(detail.Fields, sets); err != nil {
					return err
				}
				if cmd.Flags().Changed("bot-name") {
					namePtr = &botName
				}
			} else {
				if !d.Prompter.Interactive() {
					flags := "--bot-name, --enable"
					if detail.isEngine() {
						flags = "--enable"
					}
					return fmt.Errorf("Not running in a terminal; pass the settings with --set key=value (and %s).", flags)
				}
				if namePtr, cfg, err = collectSettings(detail, d.Prompter, d.RunSetup, d.Out); err != nil {
					return err
				}
			}
			if err := c.UpdateSettings(ctx, id, namePtr, cfg); err != nil {
				return err
			}
			fmt.Fprintf(d.Out, "Saved settings of %s\n", id)
			if detail.Enabled {
				return waitRunning(ctx, c, id, d.Out, d.Interval, d.RunTimeout)
			}
			if !enable && d.Prompter.Interactive() && len(sets) == 0 && !cmd.Flags().Changed("bot-name") {
				if enable, err = d.Prompter.Confirm(fmt.Sprintf("Enable %s now?", id), true); err != nil {
					return err
				}
			}
			if !enable {
				fmt.Fprintf(d.Out, "Enable it later with: openbee plugin enable %s\n", id)
				return nil
			}
			if err := c.Enable(ctx, id); err != nil {
				return err
			}
			return waitRunning(ctx, c, id, d.Out, d.Interval, d.RunTimeout)
		},
	}
	cmd.Flags().StringArrayVar(&sets, "set", nil, "Set a value without prompting, as key=value (repeatable)")
	cmd.Flags().StringVar(&botName, "bot-name", "", "Bot name users @mention in group chats")
	cmd.Flags().BoolVar(&enable, "enable", false, "Enable the plugin after saving")
	return cmd
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
