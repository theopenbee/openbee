package plugincmd

import (
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

type deps struct {
	Out        io.Writer
	Prompter   Prompter
	Lang       string
	Interval   time.Duration
	OpTimeout  time.Duration
	RunTimeout time.Duration
	NewClient  func(cfgPath string) (*Client, error)
	RunSetup   func(path, lang string) (map[string]any, error)
	Getenv     func(string) string
}

func NewCommand() *cobra.Command {
	return newCommand(deps{
		Out:        os.Stdout,
		Prompter:   surveyPrompter{},
		Interval:   time.Second,
		OpTimeout:  10 * time.Minute,
		RunTimeout: 30 * time.Second,
		NewClient: func(cfgPath string) (*Client, error) {
			return newClientFromConfig(cfgPath, os.Getenv)
		},
		RunSetup: runLocalSetup,
		Getenv:   os.Getenv,
	})
}

func newCommand(dd deps) *cobra.Command {
	d := &dd
	var cfgPath string
	root := &cobra.Command{
		Use:   "plugin",
		Short: i18n.M.Cmd.Plugin.Short,
	}
	root.PersistentFlags().StringVarP(&cfgPath, "config", "c", "config.yaml", i18n.M.Flag.ConfigPath)
	client := func() (*Client, error) {
		if d.Lang == "" {
			d.Lang = loadLanguage(cfgPath)
		}
		return d.NewClient(cfgPath)
	}
	root.AddCommand(
		d.newSearchCommand(client),
		d.newInstallCommand(client),
		d.newUpdateCommand(client),
		d.newConfigureCommand(client),
	)
	return root
}

func loadLanguage(cfgPath string) string {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return i18n.LangEN
	}
	return cfg.Language
}
