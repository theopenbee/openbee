package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

type CardAction int

const (
	ActionInstall CardAction = iota
	ActionInstalling
	ActionUpdate
	ActionConfigure
	ActionIncompatible
)

type FieldValues map[string]string

var intPattern = regexp.MustCompile(`^-?[0-9]+$`)

func FilterCatalog(c api.PluginCatalog, tag, query string) []api.CatalogPlugin {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []api.CatalogPlugin
	for _, p := range c.Plugins {
		if tag != "" && !slices.Contains(p.Tags, tag) {
			continue
		}
		if q == "" || matchesQuery(p, q) {
			out = append(out, p)
		}
	}
	return out
}

func matchesQuery(p api.CatalogPlugin, q string) bool {
	if strings.Contains(strings.ToLower(p.ID), q) {
		return true
	}
	for _, text := range []api.LocalizedText{p.Name, p.Description} {
		for _, s := range text {
			if strings.Contains(strings.ToLower(s), q) {
				return true
			}
		}
	}
	return false
}

func UsedTags(c api.PluginCatalog) []api.CatalogTag {
	var out []api.CatalogTag
	for _, tag := range c.Tags {
		used := slices.ContainsFunc(c.Plugins, func(p api.CatalogPlugin) bool {
			return slices.Contains(p.Tags, tag.ID)
		})
		if used {
			out = append(out, tag)
		}
	}
	return out
}

func CatalogAction(p api.CatalogPlugin, installing bool) CardAction {
	switch {
	case installing:
		return ActionInstalling
	case p.Installed && p.UpdateAvailable && p.Installable:
		return ActionUpdate
	case p.Installed:
		return ActionConfigure
	case p.Installable:
		return ActionInstall
	}
	return ActionIncompatible
}

func PluginName(id string, catalog *api.PluginCatalog, installed []api.PluginStatus, lang string) string {
	var names api.LocalizedText
	found := false
	if catalog != nil {
		for _, p := range catalog.Plugins {
			if p.ID == id {
				names, found = p.Name, true
				break
			}
		}
	}
	if !found {
		for _, p := range installed {
			if p.ID == id {
				names = p.Name
				break
			}
		}
	}
	if name := localize(names, lang); name != "" {
		return name
	}
	return id
}

func PluginNeedsPolling(ps []api.PluginStatus) bool {
	return slices.ContainsFunc(ps, func(p api.PluginStatus) bool {
		return p.State == "starting" || p.State == "retrying"
	})
}

func OperationsPending(ops []api.PluginOperation) bool {
	return slices.ContainsFunc(ops, operationPending)
}

func operationPending(op api.PluginOperation) bool {
	return op.Status == "queued" || op.Status == "running"
}

func SpecPackage(spec string) string {
	if at := strings.LastIndex(spec, "@"); at > 0 {
		return spec[:at]
	}
	return spec
}

func PendingInstalls(ops []api.PluginOperation) map[string]api.PluginOperation {
	out := map[string]api.PluginOperation{}
	for _, op := range ops {
		if op.Kind != "install" || op.Spec == "" || !operationPending(op) {
			continue
		}
		pkg := SpecPackage(op.Spec)
		if _, ok := out[pkg]; !ok {
			out[pkg] = op
		}
	}
	return out
}

func InitialFieldValues(d api.PluginDetail, random func() string) (FieldValues, map[string]bool) {
	if random == nil {
		random = RandomHex16
	}
	values := FieldValues{}
	bools := map[string]bool{}
	for _, f := range d.Fields {
		saved := hasValue(d.Config, f.Key)
		cur := d.Config[f.Key]
		switch f.Type {
		case "secret":
			values[f.Key] = ""
		case "bool":
			bools[f.Key] = savedOrDefaultBool(f, d.Config)
		case "int":
			values[f.Key] = ""
			if saved {
				values[f.Key] = scalarString(cur)
			}
		case "list":
			items, _ := stringList(cur)
			values[f.Key] = strings.Join(items, "\n")
		default:
			switch {
			case saved:
				values[f.Key] = scalarString(cur)
			case f.RandomDefault && !hasKey(d.Config, f.Key):
				values[f.Key] = random()
			default:
				values[f.Key] = ""
			}
		}
	}
	return values, bools
}

func ValidateInts(fields []api.PluginConfigField, v FieldValues) []string {
	var invalid []string
	for _, f := range fields {
		if f.Type != "int" {
			continue
		}
		if s := strings.TrimSpace(v[f.Key]); s != "" && !intPattern.MatchString(s) {
			invalid = append(invalid, f.Key)
		}
	}
	return invalid
}

func BuildSettingsPayload(d api.PluginDetail, v FieldValues, bools map[string]bool, botName string) (api.PluginSettingsUpdate, bool) {
	config := map[string]any{}
	for _, f := range d.Fields {
		if value, send := fieldUpdate(f, d.Config, v[f.Key], bools[f.Key]); send {
			config[f.Key] = value
		}
	}
	update := api.PluginSettingsUpdate{Config: config}
	if name := strings.TrimSpace(botName); name != d.BotName {
		update.BotName = &name
	}
	return update, len(config) > 0 || update.BotName != nil
}

func fieldUpdate(f api.PluginConfigField, config map[string]any, raw string, on bool) (any, bool) {
	saved := hasValue(config, f.Key)
	cur := config[f.Key]
	switch f.Type {
	case "secret":
		s := strings.TrimSpace(raw)
		return s, s != ""
	case "bool":
		return on, on != savedOrDefaultBool(f, config)
	case "int":
		return intUpdate(strings.TrimSpace(raw), saved, cur)
	case "list":
		next := splitLines(raw)
		prev, _ := stringList(cur)
		return next, (saved && !slices.Equal(next, prev)) || (!saved && len(next) > 0)
	}
	if saved {
		return raw, raw != scalarString(cur)
	}
	return raw, (f.RandomDefault && !hasKey(config, f.Key)) || raw != ""
}

func intUpdate(s string, saved bool, cur any) (any, bool) {
	if s == "" {
		return nil, saved
	}
	var value any
	var number float64
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		value, number = n, float64(n)
	} else if f, err := strconv.ParseFloat(s, 64); err == nil {
		value, number = f, f
	} else {
		return nil, false
	}
	return value, !saved || number != numberOf(cur)
}

func savedOrDefaultBool(f api.PluginConfigField, config map[string]any) bool {
	if hasValue(config, f.Key) {
		return truthy(config[f.Key])
	}
	return truthy(f.Default)
}

func hasKey(config map[string]any, key string) bool {
	_, ok := config[key]
	return ok
}

func hasValue(config map[string]any, key string) bool {
	return config[key] != nil
}

func splitLines(text string) []string {
	lines := []string{}
	for line := range strings.SplitSeq(text, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			lines = append(lines, s)
		}
	}
	return lines
}

func stringList(v any) ([]string, bool) {
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = scalarString(item)
	}
	return out, true
}

func scalarString(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	return fmt.Sprint(v)
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0 && !math.IsNaN(x)
	case int:
		return x != 0
	case int64:
		return x != 0
	}
	return true
}

func numberOf(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		if s := strings.TrimSpace(x); s == "" {
			return 0
		} else if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	return math.NaN()
}

func RandomHex16() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func localize(t api.LocalizedText, lang string) string {
	if len(t) == 0 {
		return ""
	}
	if text, ok := t[lang]; ok {
		return text
	}
	base := baseLanguage(lang)
	if text, ok := t[base]; ok {
		return text
	}
	keys := make([]string, 0, len(t))
	for key := range t {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if baseLanguage(key) == base {
			return t[key]
		}
	}
	if text, ok := t["en"]; ok {
		return text
	}
	return t[keys[0]]
}

func baseLanguage(tag string) string {
	base, _, _ := strings.Cut(strings.ReplaceAll(tag, "_", "-"), "-")
	return strings.ToLower(base)
}
