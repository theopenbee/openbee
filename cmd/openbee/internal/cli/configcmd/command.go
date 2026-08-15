package configcmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"

	"github.com/spf13/cobra"
)

// errInterrupted is returned when a survey prompt is cancelled (Ctrl+C).
// It is used as a sentinel so callers can suppress the error and return nil
// from the cobra RunE function.
var errInterrupted = errors.New("interrupted")

var configTemplate = config.ConfigTemplate

var configOutputPath string

// NewCommand constructs the `config` cobra command. i18n must already be loaded.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: i18n.M.Cmd.Config.Short,
		RunE: func(_ *cobra.Command, _ []string) error {
			err := runConfig()
			if errors.Is(err, errInterrupted) {
				return nil
			}
			return err
		},
	}
	cmd.Flags().StringVarP(&configOutputPath, "output", "o", "config.yaml", i18n.M.Flag.ConfigOutput)
	return cmd
}

func runConfig() error {
	vals := configValues{
		ServerPort:             "8080",
		ServerHost:             "localhost",
		DBPath:                 "./data/openbee.db",
		EngineDefault:          "claude",
		EngineTimeoutBee:       "5m",
		EngineTimeoutWorker:    "30m",
		ClaudeEnabled:          true,
		ClaudePath:             "claude",
		CodexPath:              "codex",
		PiPath:                 "pi",
		RPCTokenTTL:            "48h",
		FeederMaxConcurrentBee: 5,
		MessageDebounce:        "300ms",
		FFprobePath:            "ffprobe",
		FFmpegPath:             "ffmpeg",
		AuthUsername:           "admin",
		AuthAccessTTL:          "2h",
		AuthRefreshTTL:         "168h",
		LinearMaxMediaSize:     strconv.Itoa(50 * 1024 * 1024),
	}

	// If an existing config file exists, load its values as defaults silently
	// (do NOT print anything yet — language hasn't been selected).
	existingFound := false
	if existing := loadExistingConfig(configOutputPath); existing != nil {
		existingFound = true
		vals = *existing
	}

	// Language selection — always shown first, before all other prompts
	lang, err := runLanguageStep(vals.Language)
	if err != nil {
		return err
	}
	vals.Language = lang

	// Now print the "found existing" message using the selected locale
	if existingFound {
		fmt.Printf(i18n.M.Output.Config.FoundExisting+"\n", configOutputPath)
	}

	// Step 1 — Engine config
	fmt.Println(i18n.M.Output.Config.SectionEngine)

	enabledByName := map[string]bool{
		ai.EngineClaude: vals.ClaudeEnabled,
		ai.EngineCodex:  vals.CodexEnabled,
		ai.EnginePi:     vals.PiEnabled,
	}
	var defaultEngines []string
	for _, name := range ai.AllEngines() {
		if enabledByName[name] {
			defaultEngines = append(defaultEngines, engineLabel(name))
		}
	}
	if len(defaultEngines) == 0 {
		defaultEngines = []string{engineLabel(ai.EngineClaude)}
	}

	mappings := engineMappings()
	allEngineLabels := make([]string, len(mappings))
	for i, m := range mappings {
		allEngineLabels[i] = m.label
	}

	var selectedEngines []string
	if err := survey.AskOne(&survey.MultiSelect{
		Message: i18n.M.Prompt.EngineSelect,
		Options: allEngineLabels,
		Default: defaultEngines,
	}, &selectedEngines, survey.WithValidator(func(ans any) error {
		if v, ok := ans.([]survey.OptionAnswer); ok && len(v) == 0 {
			return errors.New("select at least one engine")
		}
		return nil
	})); err != nil {
		return handleSurveyErr(err)
	}

	vals.ClaudeEnabled = false
	vals.CodexEnabled = false
	vals.PiEnabled = false

	for _, e := range selectedEngines {
		switch engineName(e) {
		case ai.EngineClaude:
			vals.ClaudeEnabled = true
			if err := configureClaudeExecutable(&vals); err != nil {
				return err
			}
		case ai.EngineCodex:
			vals.CodexEnabled = true
			if err := configureCodexExecutable(&vals); err != nil {
				return err
			}
		case ai.EnginePi:
			vals.PiEnabled = true
			if err := configurePiExecutable(&vals); err != nil {
				return err
			}
		}
	}

	// Select default engine from enabled ones.
	// Find the label for the current default if it is still in the selection; otherwise fall back to first.
	currentDefaultLabel := engineLabel(vals.EngineDefault)
	defaultEngineOpt := selectedEngines[0]
	for _, e := range selectedEngines {
		if e == currentDefaultLabel {
			defaultEngineOpt = e
			break
		}
	}

	var selectedDefault string
	if len(selectedEngines) == 1 {
		selectedDefault = selectedEngines[0]
	} else {
		if err := survey.AskOne(&survey.Select{
			Message: i18n.M.Prompt.EngineDefault,
			Options: selectedEngines,
			Default: defaultEngineOpt,
		}, &selectedDefault); err != nil {
			return handleSurveyErr(err)
		}
	}

	vals.EngineDefault = engineName(selectedDefault)

	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.EngineTimeoutBee,
		Default: vals.EngineTimeoutBee,
	}, &vals.EngineTimeoutBee); err != nil {
		return handleSurveyErr(err)
	}

	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.EngineTimeoutWorker,
		Default: vals.EngineTimeoutWorker,
	}, &vals.EngineTimeoutWorker); err != nil {
		return handleSurveyErr(err)
	}

	installBuiltinSkills()

	// Step 2 — Platform config
	fmt.Println(i18n.M.Output.Config.SectionPlatform)

	if err := runPlatformStep(&vals); err != nil {
		return err
	}

	// Step 3 — Web Authentication
	fmt.Println(i18n.M.Output.Config.SectionAuth)

	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.Username,
		Default: vals.AuthUsername,
	}, &vals.AuthUsername, survey.WithValidator(survey.Required)); err != nil {
		return handleSurveyErr(err)
	}

	if vals.AuthPassword != "" {
		var changePassword bool
		if err := survey.AskOne(&survey.Confirm{
			Message: i18n.M.Prompt.PasswordChangeConfirm,
			Default: false,
		}, &changePassword); err != nil {
			return handleSurveyErr(err)
		}
		if changePassword {
			if err := promptPassword(&vals); err != nil {
				return err
			}
		}
	} else {
		if err := promptPassword(&vals); err != nil {
			return err
		}
	}

	// Step 4 — Advanced config
	fmt.Println(i18n.M.Output.Config.SectionAdvanced)

	var customAdvanced bool
	if err := survey.AskOne(&survey.Confirm{
		Message: i18n.M.Prompt.AdvancedConfirm,
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

	// Step 4 — Confirm write
	fmt.Println(i18n.M.Output.Config.SectionWrite)
	fmt.Printf(i18n.M.Output.Config.OutputFile+"\n", configOutputPath)

	var confirmWrite bool
	if err := survey.AskOne(&survey.Confirm{
		Message: i18n.M.Prompt.ConfirmWrite,
		Default: true,
	}, &confirmWrite); err != nil {
		return handleSurveyErr(err)
	}
	if !confirmWrite {
		fmt.Println(i18n.M.Output.Config.WriteCancelled)
		return nil
	}

	tmpl, err := template.New("config").Parse(configTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	render := configRenderData{
		configValues:       vals,
		LinearProjectsYAML: renderInlineYAMLList(vals.LinearProjects),
		LinearStatesYAML:   renderInlineYAMLList(vals.LinearStates),
	}
	if err := tmpl.Execute(&buf, render); err != nil {
		return fmt.Errorf("render template: %w", err)
	}

	if err := os.WriteFile(configOutputPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	fmt.Printf(i18n.M.Output.Config.Written+"\n", configOutputPath)
	return nil
}
