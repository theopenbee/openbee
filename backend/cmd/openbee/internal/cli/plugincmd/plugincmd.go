package plugincmd

import (
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
)

const cliLang = "en"

type deps struct {
	Out        io.Writer
	Prompter   Prompter
	Interval   time.Duration
	OpTimeout  time.Duration
	RunTimeout time.Duration
	NewClient  func(cfgPath string) (*Client, error)
	RunSetup   func(path string) (map[string]any, error)
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
		Use:          "plugin",
		Short:        "Manage plugins",
		SilenceUsage: true,
	}
	root.PersistentFlags().StringVarP(&cfgPath, "config", "c", "config.yaml", "path to config file")
	client := func() (*Client, error) {
		if d.Getenv("OPENBEE_API_KEY") != "" {
			return nil, errAgentEnvironment
		}
		return d.NewClient(cfgPath)
	}
	root.AddCommand(
		d.newSearchCommand(client),
		d.newInstallCommand(client),
		d.newConfigureCommand(client),
		d.newListCommand(client),
		d.newEnableCommand(client),
		d.newDisableCommand(client),
		d.newRestartCommand(client),
		d.newUpdateCommand(client),
		d.newRemoveCommand(client),
	)
	for _, sub := range root.Commands() {
		run := sub.RunE
		sub.RunE = func(cmd *cobra.Command, args []string) error {
			return friendlyError(run(cmd, args), cfgPath)
		}
	}
	return root
}
