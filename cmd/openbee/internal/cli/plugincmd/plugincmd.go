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
	rt := plugin.NewRuntimeResolver()
	rt.OnInstall = func(version, dir string) {
		fmt.Fprintf(os.Stdout, i18n.M.Output.Plugin.InstallingBun+"\n", version, dir)
	}
	return newCommand(deps{
		Installer: &plugin.Installer{Registry: plugin.OpenRegistry(plugin.DefaultDir()), Runtime: rt, Run: plugin.ExecRunner},
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
			lang := loadLanguage(cfgPath)
			w := tabwriter.NewWriter(d.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, i18n.M.Output.Plugin.ListHeader)
			for _, e := range entries {
				name := e.ID
				if m, err := plugin.LoadManifest(e.Path); err == nil {
					name = m.DisplayName(lang)
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.ID, name, displayVersion(e.Version), e.Source, e.Path)
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
			return nil
		},
	})
	return root
}

func loadLanguage(cfgPath string) string {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return i18n.LangEN
	}
	return cfg.Language
}

func displayVersion(v string) string {
	if v == "" {
		return "-"
	}
	return v
}
