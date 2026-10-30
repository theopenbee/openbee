package configcmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/theopenbee/openbee/internal/infra/config"

	"github.com/spf13/cobra"
)

// errInterrupted is returned when a survey prompt is cancelled (Ctrl+C).
// It is used as a sentinel so callers can suppress the error and return nil
// from the cobra RunE function.
var errInterrupted = errors.New("interrupted")

var configTemplate = config.ConfigTemplate

var configOutputPath string

var (
	askOne        = survey.AskOne
	installSkills = installBuiltinSkills
)

// NewCommand constructs the `config` cobra command.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Interactively generate a config file",
		RunE: func(_ *cobra.Command, _ []string) error {
			err := runConfig()
			if errors.Is(err, errInterrupted) {
				return nil
			}
			return err
		},
	}
	cmd.Flags().StringVarP(&configOutputPath, "output", "o", "config.yaml", "output config file path")
	return cmd
}

func defaultValues() configValues {
	return configValues{
		ServerPort:             "8080",
		ServerHost:             "localhost",
		DBPath:                 "./data/openbee.db",
		EngineTimeoutBee:       "5m",
		EngineTimeoutWorker:    "30m",
		RPCTokenTTL:            "48h",
		FeederMaxConcurrentBee: 5,
		MessageDebounce:        "300ms",
		AuthAccessTTL:          "2h",
		AuthRefreshTTL:         "168h",
	}
}

func renderConfig(vals configValues) ([]byte, error) {
	tmpl, err := template.New("config").Parse(configTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vals); err != nil {
		return nil, fmt.Errorf("render template: %w", err)
	}
	return buf.Bytes(), nil
}

func runEngineStep(vals *configValues) error {
	fmt.Println("\n=== Engine Configuration ===")
	if err := askOne(&survey.Input{
		Message: "Bee timeout:",
		Default: vals.EngineTimeoutBee,
	}, &vals.EngineTimeoutBee); err != nil {
		return handleSurveyErr(err)
	}
	if err := askOne(&survey.Input{
		Message: "Worker timeout:",
		Default: vals.EngineTimeoutWorker,
	}, &vals.EngineTimeoutWorker); err != nil {
		return handleSurveyErr(err)
	}
	installSkills()
	return nil
}

func runConfig() error {
	vals := defaultValues()

	if existing := loadExistingConfig(configOutputPath); existing != nil {
		vals = *existing
		fmt.Printf("Found existing config at %s, using its values as defaults.\n", configOutputPath)
	}

	lang, err := runLanguageStep(vals.Language)
	if err != nil {
		return err
	}
	vals.Language = lang

	if err := runEngineStep(&vals); err != nil {
		return err
	}

	// Step 2 — Advanced config
	fmt.Println("\n=== Advanced Configuration ===")

	var customAdvanced bool
	if err := askOne(&survey.Confirm{
		Message: "Customize advanced settings?",
		Default: false,
	}, &customAdvanced); err != nil {
		return handleSurveyErr(err)
	}

	if customAdvanced {
		if err := runAdvancedPrompts(&vals); err != nil {
			return err
		}
	} else {
		if vals.AuthJWTSecret == "" {
			vals.AuthJWTSecret = config.GenerateRandomSecret()
		}
		if vals.RPCTokenSecret == "" {
			vals.RPCTokenSecret = config.GenerateRandomSecret()
		}
		if vals.ServerEnvSecret == "" {
			vals.ServerEnvSecret = config.GenerateRandomSecret()
		}
	}

	// Step 3 — Confirm write
	fmt.Println("\n=== Write Configuration ===")
	fmt.Printf("Output file: %s\n", configOutputPath)

	var confirmWrite bool
	if err := askOne(&survey.Confirm{
		Message: "Confirm write config file?",
		Default: true,
	}, &confirmWrite); err != nil {
		return handleSurveyErr(err)
	}
	if !confirmWrite {
		fmt.Println("Write cancelled.")
		return nil
	}

	rendered, err := renderConfig(vals)
	if err != nil {
		return err
	}

	if err := os.WriteFile(configOutputPath, rendered, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	fmt.Printf("Config file written to: %s\n", configOutputPath)
	fmt.Println("Next: start openbee (openbee server -d), create the admin account in the web console, then add chat channels with: openbee plugin search")
	return nil
}
