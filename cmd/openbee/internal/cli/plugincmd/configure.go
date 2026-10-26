package plugincmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/i18n"
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
			return nil, fmt.Errorf(i18n.M.Output.Plugin.SetInvalid, set)
		}
		switch types[key] {
		case plugin.FieldBool:
			b, err := strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf(i18n.M.Output.Plugin.SetType, key, err)
			}
			out[key] = b
		case plugin.FieldInt:
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf(i18n.M.Output.Plugin.SetType, key, err)
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

func fieldLabel(f plugin.ConfigField, lang string) string {
	if l := f.Label.Get(lang); l != "" {
		return l
	}
	return f.Key
}

func collectSettings(d Detail, p Prompter, lang string, runSetup func(path, lang string) (map[string]any, error), out io.Writer) (*string, map[string]any, error) {
	m := i18n.M.Output.Plugin
	cfg := map[string]any{}
	if d.HasSetup {
		label := d.SetupLabel.Get(lang)
		if label == "" {
			label = i18n.M.Prompt.PluginSetup
		}
		run, err := p.Confirm(fmt.Sprintf(m.SetupRun, label), missingRequired(d))
		if err != nil {
			return nil, nil, err
		}
		if run {
			values, err := runSetup(d.Path, lang)
			if err != nil {
				fmt.Fprintf(out, m.SetupFailed+"\n", label, err)
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
		current, hasCur := d.Config[f.Key]
		hasCur = hasCur && hasValue(current)
		label := fieldLabel(f, lang)
		help := f.Help.Get(lang)
		switch f.Type {
		case plugin.FieldSecret:
			if hasCur {
				help = strings.TrimSpace(help + " " + fmt.Sprintf(m.KeepSecret, current))
			}
			for {
				v, err := p.Password(label, help)
				if err != nil {
					return nil, nil, err
				}
				if v != "" {
					cfg[f.Key] = v
					break
				}
				if hasCur || !f.Required {
					break
				}
				fmt.Fprintf(out, m.FieldRequired+"\n", label)
			}
		case plugin.FieldBool:
			def, _ := current.(bool)
			if !hasCur {
				def, _ = f.Default.(bool)
			}
			v, err := p.Confirm(label, def)
			if err != nil {
				return nil, nil, err
			}
			cfg[f.Key] = v
		case plugin.FieldInt:
			def := textDefault(current)
			if !hasCur {
				def = textDefault(f.Default)
			}
			v, err := p.Input(label, help, def, func(s string) error {
				if s == "" && !f.Required {
					return nil
				}
				if _, err := strconv.Atoi(strings.TrimSpace(s)); err != nil {
					return errors.New(m.IntInvalid)
				}
				return nil
			})
			if err != nil {
				return nil, nil, err
			}
			if v = strings.TrimSpace(v); v != "" {
				n, _ := strconv.Atoi(v)
				cfg[f.Key] = n
			}
		case plugin.FieldList:
			help = strings.TrimSpace(help + " " + i18n.M.Prompt.PluginListHint)
			v, err := p.Input(label, help, textDefault(current), nil)
			if err != nil {
				return nil, nil, err
			}
			if v != "" || hasCur {
				cfg[f.Key] = splitList(v)
			}
		default:
			def := textDefault(current)
			if !hasCur && !f.RandomDefault {
				def = textDefault(f.Default)
			}
			if f.RandomDefault && !hasCur {
				help = strings.TrimSpace(help + " " + m.RandomDefault)
			}
			required := f.Required && !hasCur
			v, err := p.Input(label, help, def, func(s string) error {
				if required && strings.TrimSpace(s) == "" {
					return fmt.Errorf(m.FieldRequired, label)
				}
				return nil
			})
			if err != nil {
				return nil, nil, err
			}
			if v != "" || hasCur {
				cfg[f.Key] = v
			}
		}
	}
	name, err := p.Input(m.BotNamePrompt, m.BotNameHelp, d.BotName, nil)
	if err != nil {
		return nil, nil, err
	}
	name = strings.TrimSpace(name)
	if name == d.BotName {
		return nil, cfg, nil
	}
	return &name, cfg, nil
}

func waitRunning(ctx context.Context, c *Client, id string, out io.Writer, interval, timeout time.Duration) error {
	m := i18n.M.Output.Plugin
	deadline := time.Now().Add(timeout)
	for {
		d, err := c.Get(ctx, id)
		if err != nil {
			return err
		}
		switch {
		case d.State == "running":
			fmt.Fprintf(out, m.Started+"\n", id)
			return nil
		case d.State == "failed", d.State == "retrying" && d.Error != "":
			return fmt.Errorf(m.StartFailed, id, d.Error)
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(out, m.StillStarting+"\n", id)
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
		Short: i18n.M.Cmd.Plugin.Sub("configure"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m := i18n.M.Output.Plugin
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
					return errors.New(m.NeedTerminal)
				}
				if namePtr, cfg, err = collectSettings(detail, d.Prompter, d.Lang, d.RunSetup, d.Out); err != nil {
					return err
				}
			}
			if err := c.UpdateSettings(ctx, id, namePtr, cfg); err != nil {
				return err
			}
			fmt.Fprintf(d.Out, m.Saved+"\n", id)
			if detail.Enabled {
				return waitRunning(ctx, c, id, d.Out, d.Interval, d.RunTimeout)
			}
			if !enable && d.Prompter.Interactive() && len(sets) == 0 && !cmd.Flags().Changed("bot-name") {
				if enable, err = d.Prompter.Confirm(fmt.Sprintf(m.EnableNow, id), true); err != nil {
					return err
				}
			}
			if !enable {
				fmt.Fprintf(d.Out, m.EnableLater+"\n", id)
				return nil
			}
			if err := c.Enable(ctx, id); err != nil {
				return err
			}
			return waitRunning(ctx, c, id, d.Out, d.Interval, d.RunTimeout)
		},
	}
	cmd.Flags().StringArrayVar(&sets, "set", nil, i18n.M.Output.Plugin.SetFlag)
	cmd.Flags().StringVar(&botName, "bot-name", "", i18n.M.Output.Plugin.BotNameFlag)
	cmd.Flags().BoolVar(&enable, "enable", false, i18n.M.Output.Plugin.EnableFlag)
	return cmd
}
