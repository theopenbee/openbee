package configcmd

import (
	"fmt"
	"slices"

	"github.com/AlecAivazis/survey/v2"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/plugin"
)

// runPlatformStep runs the multi-select prompt and per-platform configuration.
func runPlatformStep(vals *configValues) error {
	plugins := loadPluginOptions(plugin.OpenRegistry(plugin.DefaultDir()), vals.Language)
	if len(plugins) == 0 {
		fmt.Println(i18n.M.Prompt.PlatformNone)
		return nil
	}

	options := make([]string, 0, len(plugins))
	var defaults []string
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
