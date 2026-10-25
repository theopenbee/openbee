package configcmd

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"gopkg.in/yaml.v3"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
	"github.com/theopenbee/openbee/internal/plugin"
)

type pluginOption struct {
	Manifest plugin.Manifest
	Label    string
}

func loadPluginOptions(reg *plugin.Registry, lang string) []pluginOption {
	entries, err := reg.List()
	if err != nil {
		return nil
	}
	var out []pluginOption
	for _, e := range entries {
		m, err := plugin.LoadManifest(e.Path)
		if err != nil || m.ID != e.ID {
			continue
		}
		out = append(out, pluginOption{Manifest: m, Label: m.DisplayName(lang) + i18n.M.Prompt.PluginSuffix})
	}
	return out
}

func renderPluginSections(sections map[string]map[string]any) (string, error) {
	ids := make([]string, 0, len(sections))
	for id := range sections {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var lines []string
	for _, id := range ids {
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		section := sections[id]
		if section == nil {
			section = map[string]any{}
		}
		if err := enc.Encode(map[string]any{id: section}); err != nil {
			return "", fmt.Errorf("render plugin section %q: %w", id, err)
		}
		if err := enc.Close(); err != nil {
			return "", err
		}
		for _, l := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
			lines = append(lines, "    "+l)
		}
	}
	return strings.Join(lines, "\n"), nil
}

func applyPluginSelection(vals *configValues, offered, selected []string) {
	for _, id := range offered {
		if slices.Contains(selected, id) {
			continue
		}
		if section, ok := vals.PluginSections[id]; ok {
			section["enabled"] = false
		}
	}
}

func promptPluginSection(m plugin.Manifest, lang string, current map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(current)+2)
	for k, v := range current {
		out[k] = v
	}
	for _, f := range m.Config {
		v, err := promptPluginField(f, lang, out[f.Key])
		if err != nil {
			return nil, err
		}
		if v == nil {
			delete(out, f.Key)
			continue
		}
		out[f.Key] = v
	}
	botName, _ := out["bot_name"].(string)
	if err := promptBotName(&botName); err != nil {
		return nil, err
	}
	out["bot_name"] = botName
	out["enabled"] = true
	return out, nil
}

func promptPluginField(f plugin.ConfigField, lang string, current any) (any, error) {
	if current == nil {
		current = f.Default
	}
	label := f.Label.Get(lang)
	if label == "" {
		label = f.Key
	}
	help := f.Help.Get(lang)

	switch f.Type {
	case plugin.FieldBool:
		def, _ := current.(bool)
		var v bool
		err := survey.AskOne(&survey.Confirm{Message: label, Help: help, Default: def}, &v)
		return v, handleSurveyErr(err)
	case plugin.FieldSecret:
		existing, _ := current.(string)
		var opts []survey.AskOpt
		if f.Required && existing == "" {
			opts = append(opts, survey.WithValidator(survey.Required))
		}
		var v string
		if err := survey.AskOne(&survey.Password{Message: label, Help: help}, &v, opts...); err != nil {
			return nil, handleSurveyErr(err)
		}
		if v == "" {
			return nilIfEmpty(existing), nil
		}
		return v, nil
	case plugin.FieldInt:
		def := ""
		switch n := current.(type) {
		case int:
			def = strconv.Itoa(n)
		case float64:
			def = strconv.Itoa(int(n))
		}
		var v string
		opts := []survey.AskOpt{survey.WithValidator(func(ans any) error {
			s, _ := ans.(string)
			if s == "" && !f.Required {
				return nil
			}
			if _, err := strconv.Atoi(strings.TrimSpace(s)); err != nil {
				return errors.New("enter an integer")
			}
			return nil
		})}
		if err := survey.AskOne(&survey.Input{Message: label, Help: help, Default: def}, &v, opts...); err != nil {
			return nil, handleSurveyErr(err)
		}
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n, nil
	case plugin.FieldList:
		def := strings.Join(toStrings(current), ",")
		if help == "" {
			help = i18n.M.Prompt.PluginListHint
		}
		var opts []survey.AskOpt
		if f.Required {
			opts = append(opts, survey.WithValidator(survey.Required))
		}
		var v string
		if err := survey.AskOne(&survey.Input{Message: label, Help: help, Default: def}, &v, opts...); err != nil {
			return nil, handleSurveyErr(err)
		}
		return utils.SplitAndTrim(v), nil
	default:
		def, _ := current.(string)
		if def == "" && f.RandomDefault {
			def = randomHex(8)
		}
		var opts []survey.AskOpt
		if f.Required {
			opts = append(opts, survey.WithValidator(survey.Required))
		}
		var v string
		if err := survey.AskOne(&survey.Input{Message: label, Help: help, Default: def}, &v, opts...); err != nil {
			return nil, handleSurveyErr(err)
		}
		return nilIfEmpty(v), nil
	}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func toStrings(v any) []string {
	switch items := v.(type) {
	case []string:
		return items
	case []any:
		out := make([]string, 0, len(items))
		for _, it := range items {
			if s, ok := it.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
