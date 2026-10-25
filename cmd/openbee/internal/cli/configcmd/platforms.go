package configcmd

import (
	"fmt"
	"slices"

	"github.com/AlecAivazis/survey/v2"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/plugin"
)

type platformSpec struct {
	enabled   *bool
	label     string
	configure func(*configValues) error
}

func platformSpecs(vals *configValues) []platformSpec {
	return []platformSpec{
		{&vals.LinearEnabled, i18n.M.Prompt.PlatformLinear, configureLinear},
	}
}

func configureLinear(vals *configValues) error {
	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.LinearAPIKey,
		Help:    i18n.M.Prompt.LinearAPIKeyHelp,
		Default: vals.LinearAPIKey,
	}, &vals.LinearAPIKey, survey.WithValidator(survey.Required)); err != nil {
		return handleSurveyErr(err)
	}
	labelDefault := vals.LinearLabelName
	if labelDefault == "" {
		labelDefault = "openbee"
	}
	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.LinearLabelName,
		Default: labelDefault,
	}, &vals.LinearLabelName); err != nil {
		return handleSurveyErr(err)
	}
	if vals.LinearPollInterval == "" {
		vals.LinearPollInterval = "10s"
	}
	fmt.Println(i18n.M.Prompt.LinearProjectsHelp)
	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.LinearProjects,
		Default: vals.LinearProjects,
	}, &vals.LinearProjects); err != nil {
		return handleSurveyErr(err)
	}
	fmt.Println(i18n.M.Prompt.LinearStatesHelp)
	return handleSurveyErr(survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.LinearStates,
		Default: vals.LinearStates,
	}, &vals.LinearStates))
}

// runPlatformStep runs the multi-select prompt and per-platform configuration.
func runPlatformStep(vals *configValues) error {
	specs := platformSpecs(vals)
	plugins := loadPluginOptions(plugin.OpenRegistry(plugin.DefaultDir()), vals.Language)

	var defaults []string
	for _, s := range specs {
		if *s.enabled {
			defaults = append(defaults, s.label)
		}
	}
	options := make([]string, 0, len(specs)+len(plugins))
	for _, s := range specs {
		options = append(options, s.label)
	}
	for _, po := range plugins {
		options = append(options, po.Label)
		if on, _ := plugin.SectionEnabled(vals.PluginSections[po.Manifest.ID]); on {
			defaults = append(defaults, po.Label)
		}
	}

	var selected []string
	if err := survey.AskOne(&survey.MultiSelect{
		Message: i18n.M.Prompt.PlatformSelect,
		Options: options,
		Default: defaults,
	}, &selected); err != nil {
		return handleSurveyErr(err)
	}

	for _, s := range specs {
		*s.enabled = false
	}

	byLabel := make(map[string]platformSpec, len(specs))
	for _, s := range specs {
		byLabel[s.label] = s
	}
	for _, p := range selected {
		spec, ok := byLabel[p]
		if !ok {
			continue
		}
		*spec.enabled = true
		if err := spec.configure(vals); err != nil {
			return err
		}
	}
	return configurePlugins(vals, plugins, selected)
}

func configurePlugins(vals *configValues, plugins []pluginOption, selectedLabels []string) error {
	offered := make([]string, 0, len(plugins))
	var selected []string
	for _, po := range plugins {
		offered = append(offered, po.Manifest.ID)
		if slices.Contains(selectedLabels, po.Label) {
			selected = append(selected, po.Manifest.ID)
		}
	}
	applyPluginSelection(vals, offered, selected)
	for _, po := range plugins {
		id := po.Manifest.ID
		if !slices.Contains(selected, id) {
			continue
		}
		fmt.Println(po.Label)
		section, err := promptPluginSection(po.Manifest, vals.Language, vals.PluginSections[id])
		if err != nil {
			return err
		}
		if vals.PluginSections == nil {
			vals.PluginSections = map[string]map[string]any{}
		}
		vals.PluginSections[id] = section
	}
	return nil
}
