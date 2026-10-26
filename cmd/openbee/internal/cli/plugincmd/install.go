package plugincmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

var (
	errNoSuchDirectory  = errors.New("no such directory")
	errNotInCatalog     = errors.New("not in the plugin catalog")
	errCatalogRequired  = errors.New("plugin catalog is required")
	errOperationTimeout = errors.New("operation is still running")
)

type resolveError struct {
	sentinel error
	msg      string
}

func (e *resolveError) Error() string { return e.msg }
func (e *resolveError) Unwrap() error { return e.sentinel }

func resolveSpec(arg string, loadCatalog func() (*Catalog, error), stat func(string) (os.FileInfo, error)) (string, error) {
	if name, ok := strings.CutPrefix(arg, "npm:"); ok {
		return name, nil
	}
	if info, err := stat(arg); err == nil && info.IsDir() {
		return filepath.Abs(arg)
	}
	if strings.HasPrefix(arg, "@") || strings.ContainsAny(arg, ":@") {
		return arg, nil
	}
	if strings.ContainsRune(arg, '/') || strings.HasPrefix(arg, ".") {
		return "", &resolveError{errNoSuchDirectory, fmt.Sprintf(i18n.M.Output.Plugin.NoSuchDirectory, arg)}
	}
	cat, err := loadCatalog()
	if errors.Is(err, ErrNotRunning) {
		return "", err
	}
	if err != nil {
		return "", &resolveError{errCatalogRequired, fmt.Sprintf(i18n.M.Output.Plugin.CatalogRequired, arg, err)}
	}
	for _, p := range cat.Plugins {
		if p.ID == arg {
			return p.Package + "@" + p.Version, nil
		}
	}
	return "", &resolveError{errNotInCatalog, fmt.Sprintf(i18n.M.Output.Plugin.NotInCatalog, arg, arg)}
}

func waitOperation(ctx context.Context, c *Client, id string, interval, timeout time.Duration) (Operation, error) {
	deadline := time.Now().Add(timeout)
	for {
		op, err := c.Operation(ctx, id)
		if err != nil {
			return Operation{}, err
		}
		if op.Status == "succeeded" || op.Status == "failed" {
			return op, nil
		}
		if time.Now().After(deadline) {
			return op, errOperationTimeout
		}
		select {
		case <-ctx.Done():
			return op, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func installAll(ctx context.Context, c *Client, specs []string, out io.Writer, interval, timeout time.Duration) ([]string, error) {
	m := i18n.M.Output.Plugin
	var installed []string
	failed := 0
	for _, spec := range specs {
		fmt.Fprintf(out, m.Installing+"\n", spec)
		op, err := c.Install(ctx, spec)
		if err == nil {
			op, err = waitOperation(ctx, c, op.ID, interval, timeout)
		}
		switch {
		case errors.Is(err, errOperationTimeout):
			failed++
			fmt.Fprintf(out, m.OpTimeout+"\n", spec)
		case err != nil:
			failed++
			fmt.Fprintf(out, m.InstallFailed+"\n", spec, err)
		case op.Status == "failed":
			failed++
			fmt.Fprintf(out, m.InstallFailed+"\n", spec, op.Error)
		default:
			installed = append(installed, op.PluginID)
			fmt.Fprintf(out, m.InstallOK+"\n", op.PluginID)
		}
	}
	if len(specs) > 1 {
		fmt.Fprintf(out, m.Summary+"\n", len(installed), failed)
	}
	if failed > 0 {
		return installed, fmt.Errorf(m.Summary, len(installed), failed)
	}
	return installed, nil
}

func printConfigureHints(out io.Writer, ids []string) {
	if len(ids) == 0 {
		return
	}
	m := i18n.M.Output.Plugin
	fmt.Fprintln(out, m.TrustWarning)
	for _, id := range ids {
		fmt.Fprintf(out, m.ConfigureHint+"\n", id)
	}
}

func installAndHint(ctx context.Context, c *Client, specs []string, out io.Writer, interval, timeout time.Duration) error {
	ids, err := installAll(ctx, c, specs, out, interval, timeout)
	printConfigureHints(out, ids)
	return err
}

func matchesKeyword(p CatalogPlugin, keyword string) bool {
	if keyword == "" {
		return true
	}
	k := strings.ToLower(keyword)
	fields := []string{p.ID, p.Package}
	for _, v := range p.Name {
		fields = append(fields, v)
	}
	for _, v := range p.Description {
		fields = append(fields, v)
	}
	fields = append(fields, p.Tags...)
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), k) {
			return true
		}
	}
	return false
}

func catalogStatus(p CatalogPlugin) string {
	m := i18n.M.Output.Plugin
	switch {
	case p.Installed && p.UpdateAvailable:
		return fmt.Sprintf(m.StatusUpdate, p.InstalledVersion, p.Version)
	case p.Installed:
		return m.StatusInstalled
	case !p.Installable:
		return m.StatusUnavailable
	default:
		return m.StatusAvailable
	}
}

func (d *deps) newSearchCommand(client func() (*Client, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "search [keyword]",
		Short: i18n.M.Cmd.Plugin.Sub("search"),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client()
			if err != nil {
				return err
			}
			cat, err := c.Catalog(cmd.Context(), false)
			if err != nil {
				return err
			}
			keyword := ""
			if len(args) == 1 {
				keyword = args[0]
			}
			m := i18n.M.Output.Plugin
			var matched, selectable []CatalogPlugin
			for _, p := range cat.Plugins {
				if !matchesKeyword(p, keyword) {
					continue
				}
				matched = append(matched, p)
				if p.Installable && !p.Installed {
					selectable = append(selectable, p)
				}
			}
			if len(matched) == 0 {
				fmt.Fprintln(d.Out, m.SearchEmpty)
				return nil
			}
			w := tabwriter.NewWriter(d.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, m.SearchHeader)
			for _, p := range matched {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.ID, p.Name.Get(d.Lang), p.Version, catalogStatus(p))
			}
			if err := w.Flush(); err != nil {
				return err
			}
			if !d.Prompter.Interactive() {
				fmt.Fprintln(d.Out, m.SearchHint)
				return nil
			}
			if len(selectable) == 0 {
				fmt.Fprintln(d.Out, m.SearchNothing)
				return nil
			}
			options := make([]string, len(selectable))
			for i, p := range selectable {
				options[i] = fmt.Sprintf("%s  %s (%s)", p.ID, p.Name.Get(d.Lang), p.Version)
			}
			picked, err := d.Prompter.MultiSelect(m.SearchSelect, options)
			if err != nil || len(picked) == 0 {
				return err
			}
			specs := make([]string, len(picked))
			for i, idx := range picked {
				specs[i] = selectable[idx].Package + "@" + selectable[idx].Version
			}
			return installAndHint(cmd.Context(), c, specs, d.Out, d.Interval, d.OpTimeout)
		},
	}
}

func (d *deps) newInstallCommand(client func() (*Client, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "install <id|npm-spec|path>...",
		Short: i18n.M.Cmd.Plugin.Sub("install"),
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client()
			if err != nil {
				return err
			}
			var cached *Catalog
			var catErr error
			loadCatalog := func() (*Catalog, error) {
				if cached == nil && catErr == nil {
					cached, catErr = c.Catalog(cmd.Context(), false)
				}
				return cached, catErr
			}
			specs := make([]string, 0, len(args))
			for _, arg := range args {
				spec, err := resolveSpec(arg, loadCatalog, os.Stat)
				if err != nil {
					return err
				}
				specs = append(specs, spec)
			}
			return installAndHint(cmd.Context(), c, specs, d.Out, d.Interval, d.OpTimeout)
		},
	}
}

func (d *deps) newUpdateCommand(client func() (*Client, error)) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "update [id...]",
		Short: i18n.M.Cmd.Plugin.Sub("update"),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client()
			if err != nil {
				return err
			}
			cat, err := c.Catalog(cmd.Context(), true)
			if err != nil {
				return err
			}
			want := map[string]bool{}
			for _, a := range args {
				want[a] = true
			}
			var specs []string
			for _, p := range cat.Plugins {
				if !p.UpdateAvailable || !p.Installable {
					continue
				}
				if len(want) > 0 && !all && !want[p.ID] {
					continue
				}
				specs = append(specs, p.Package+"@"+p.Version)
			}
			if len(specs) == 0 {
				fmt.Fprintln(d.Out, i18n.M.Output.Plugin.UpdateNone)
				return nil
			}
			ids, err := installAll(cmd.Context(), c, specs, d.Out, d.Interval, d.OpTimeout)
			for _, id := range ids {
				p, gerr := c.Get(cmd.Context(), id)
				if gerr != nil || !p.Enabled {
					continue
				}
				if werr := waitRunning(cmd.Context(), c, id, d.Out, d.Interval, d.RunTimeout); werr != nil && err == nil {
					err = werr
				}
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, i18n.M.Output.Plugin.UpdateAllFlag)
	return cmd
}
