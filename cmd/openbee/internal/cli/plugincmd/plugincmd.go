package plugincmd

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/plugin"
)

type deps struct {
	Installer *plugin.Installer
	Out       io.Writer
}

func NewCommand() *cobra.Command {
	return newCommand(deps{
		Installer: &plugin.Installer{Registry: plugin.OpenRegistry(plugin.DefaultDir()), Run: plugin.ExecRunner},
		Out:       os.Stdout,
	})
}

func newCommand(d deps) *cobra.Command {
	var cfgPath string
	root := &cobra.Command{
		Use:   "plugin",
		Short: i18n.M.Cmd.Plugin.Short,
	}
	root.PersistentFlags().StringVarP(&cfgPath, "config", "c", "config.yaml", i18n.M.Flag.ConfigPath)

	root.AddCommand(&cobra.Command{
		Use:   "install <npm-spec|path>",
		Short: i18n.M.Cmd.Plugin.Sub("install"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, m, err := d.Installer.Install(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(d.Out, i18n.M.Output.Plugin.Installed+"\n", entry.ID, displayVersion(m.Version))
			fmt.Fprintln(d.Out, i18n.M.Output.Plugin.TrustWarning)
			fmt.Fprintln(d.Out, i18n.M.Output.Plugin.NextSteps)
			return nil
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "list",
		Short: i18n.M.Cmd.Plugin.Sub("list"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := d.Installer.Registry.List()
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Fprintln(d.Out, i18n.M.Output.Plugin.ListEmpty)
				return nil
			}
			sections, lang := loadSections(cfgPath)
			w := tabwriter.NewWriter(d.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, i18n.M.Output.Plugin.ListHeader)
			for _, e := range entries {
				name := e.ID
				if m, err := plugin.LoadManifest(e.Path); err == nil {
					name = m.DisplayName(lang)
				}
				status := i18n.M.Output.Plugin.StatusDisabled
				if enabled(sections, e.ID) {
					status = i18n.M.Output.Plugin.StatusEnabled
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", e.ID, name, displayVersion(e.Version), e.Source, status, e.Path)
			}
			return w.Flush()
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "remove <id>",
		Short: i18n.M.Cmd.Plugin.Sub("remove"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := d.Installer.Remove(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(d.Out, i18n.M.Output.Plugin.Removed+"\n", entry.ID)
			if sections, _ := loadSections(cfgPath); enabled(sections, entry.ID) {
				fmt.Fprintf(d.Out, i18n.M.Output.Plugin.StillEnabled+"\n", entry.ID)
			}
			return nil
		},
	})
	return root
}

func loadSections(cfgPath string) (map[string]map[string]any, string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, i18n.LangEN
	}
	return cfg.Bee.Platforms.Plugins, cfg.Language
}

func enabled(sections map[string]map[string]any, id string) bool {
	on, _ := sections[id]["enabled"].(bool)
	return on
}

func displayVersion(v string) string {
	if v == "" {
		return "-"
	}
	return v
}
