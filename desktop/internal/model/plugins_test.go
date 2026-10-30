package model

import (
	"encoding/json"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func settingsFields() []api.PluginConfigField {
	return []api.PluginConfigField{
		{Key: "token", Type: "secret", Required: true},
		{Key: "name", Type: "string"},
		{Key: "secret_path", Type: "string", RandomDefault: true},
		{Key: "port", Type: "int", Default: float64(8080)},
		{Key: "admins", Type: "list"},
		{Key: "debug", Type: "bool", Default: true},
	}
}

func savedConfig() map[string]any {
	return map[string]any{
		"token":       "****abcd",
		"name":        "bee",
		"port":        float64(9000),
		"admins":      []any{"a"},
		"debug":       true,
		"secret_path": "x",
	}
}

func detailOf(fields []api.PluginConfigField, config map[string]any, botName string) api.PluginDetail {
	return api.PluginDetail{
		PluginStatus: api.PluginStatus{ID: "telegram", BotName: botName},
		Config:       config,
		Fields:       fields,
	}
}

func fixedRandom() string { return "0123456789abcdef" }

func TestInitialFieldValuesFillsSavedValuesAndLeavesSecretsEmpty(t *testing.T) {
	config := map[string]any{
		"token":       "****abcd",
		"name":        "bee",
		"secret_path": "",
		"port":        float64(9000),
		"admins":      []any{"a", "b"},
		"debug":       false,
	}
	values, bools := InitialFieldValues(detailOf(settingsFields(), config, ""), fixedRandom)
	wantValues := FieldValues{"token": "", "name": "bee", "secret_path": "", "port": "9000", "admins": "a\nb"}
	if !maps.Equal(values, wantValues) {
		t.Fatalf("values = %v, want %v", values, wantValues)
	}
	if want := map[string]bool{"debug": false}; !maps.Equal(bools, want) {
		t.Fatalf("bools = %v, want %v", bools, want)
	}
}

func TestInitialFieldValuesPrefillsRandomDefaultOnlyForUnsavedFields(t *testing.T) {
	values, bools := InitialFieldValues(detailOf(settingsFields(), map[string]any{}, ""), fixedRandom)
	if values["secret_path"] != "0123456789abcdef" {
		t.Fatalf("secret_path = %q, want the generated value", values["secret_path"])
	}
	if values["name"] != "" || values["port"] != "" || values["admins"] != "" || values["token"] != "" {
		t.Fatalf("other fields must stay empty: %v", values)
	}
	if !bools["debug"] {
		t.Fatal("debug must default to true")
	}
}

func TestInitialFieldValuesKeepsAnEmptiedRandomDefaultKey(t *testing.T) {
	config := map[string]any{"secret_path": nil}
	values, _ := InitialFieldValues(detailOf(settingsFields(), config, ""), fixedRandom)
	if values["secret_path"] != "" {
		t.Fatalf("secret_path = %q, want empty when the key exists with null", values["secret_path"])
	}
}

func TestInitialFieldValuesBoolFields(t *testing.T) {
	fields := []api.PluginConfigField{
		{Key: "on_default", Type: "bool", Default: true},
		{Key: "off_default", Type: "bool"},
		{Key: "saved_true", Type: "bool", Default: false},
		{Key: "saved_false", Type: "bool", Default: true},
		{Key: "null_saved", Type: "bool", Default: true},
	}
	config := map[string]any{"saved_true": true, "saved_false": false, "null_saved": nil}
	values, bools := InitialFieldValues(detailOf(fields, config, ""), fixedRandom)
	if len(values) != 0 {
		t.Fatalf("bool fields must not appear in values: %v", values)
	}
	want := map[string]bool{
		"on_default":  true,
		"off_default": false,
		"saved_true":  true,
		"saved_false": false,
		"null_saved":  true,
	}
	if !maps.Equal(bools, want) {
		t.Fatalf("bools = %v, want %v", bools, want)
	}
}

func TestInitialFieldValuesIntRendersWithoutExponent(t *testing.T) {
	fields := []api.PluginConfigField{{Key: "n", Type: "int"}}
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"small", float64(9000), "9000"},
		{"zero", float64(0), "0"},
		{"negative", float64(-3), "-3"},
		{"large without exponent", float64(10000000000000000000), "10000000000000000000"},
		{"decoded from json", decodeAny(t, `1000000000`), "1000000000"},
		{"string value", "42", "42"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			values, _ := InitialFieldValues(detailOf(fields, map[string]any{"n": tc.in}, ""), fixedRandom)
			if values["n"] != tc.want {
				t.Fatalf("got %q, want %q", values["n"], tc.want)
			}
		})
	}
}

func decodeAny(t *testing.T, raw string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestInitialFieldValuesListAndString(t *testing.T) {
	fields := []api.PluginConfigField{
		{Key: "admins", Type: "list"},
		{Key: "ids", Type: "list"},
		{Key: "other", Type: "list"},
		{Key: "greeting", Type: "string"},
	}
	config := map[string]any{
		"admins":   []any{"a", "b"},
		"ids":      []any{float64(1), float64(2)},
		"other":    "not-a-list",
		"greeting": "hi",
	}
	values, _ := InitialFieldValues(detailOf(fields, config, ""), fixedRandom)
	want := FieldValues{"admins": "a\nb", "ids": "1\n2", "other": "", "greeting": "hi"}
	if !maps.Equal(values, want) {
		t.Fatalf("got %v, want %v", values, want)
	}
}

func TestInitialFieldValuesNilRandomUsesTheDefaultGenerator(t *testing.T) {
	values, _ := InitialFieldValues(detailOf(settingsFields(), map[string]any{}, ""), nil)
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(values["secret_path"]) {
		t.Fatalf("secret_path = %q, want 16 hex characters", values["secret_path"])
	}
}

func TestValidateInts(t *testing.T) {
	fields := settingsFields()
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{"digits", "12", nil},
		{"negative", "-3", nil},
		{"empty", "", nil},
		{"padded", " -3 ", nil},
		{"zero", "0", nil},
		{"decimal", "1.5", []string{"port"}},
		{"letters", "x", []string{"port"}},
		{"trailing letter", "12a", []string{"port"}},
		{"plus sign", "+3", []string{"port"}},
		{"lone minus", "-", []string{"port"}},
		{"inner space", "1 2", []string{"port"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidateInts(fields, FieldValues{"port": tc.value}); !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidateIntsOnlyChecksIntFieldsInOrder(t *testing.T) {
	fields := []api.PluginConfigField{
		{Key: "name", Type: "string"},
		{Key: "a", Type: "int"},
		{Key: "b", Type: "int"},
	}
	got := ValidateInts(fields, FieldValues{"name": "x", "a": "no", "b": "yes"})
	if want := []string{"a", "b"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := ValidateInts(fields, nil); len(got) != 0 {
		t.Fatalf("missing values are empty and valid, got %v", got)
	}
}

func payloadFor(t *testing.T, config map[string]any, mutate func(FieldValues, map[string]bool), botName string) (api.PluginSettingsUpdate, bool) {
	t.Helper()
	d := detailOf(settingsFields(), config, "bee-bot")
	values, bools := InitialFieldValues(d, fixedRandom)
	if mutate != nil {
		mutate(values, bools)
	}
	return BuildSettingsPayload(d, values, bools, botName)
}

func TestBuildSettingsPayloadOmitsUntouchedFields(t *testing.T) {
	got, ok := payloadFor(t, savedConfig(), nil, "bee-bot")
	if ok || len(got.Config) != 0 || got.BotName != nil {
		t.Fatalf("got (%#v, %v), want nothing to send", got, ok)
	}
}

func TestBuildSettingsPayloadEmptySecretKeepsValue(t *testing.T) {
	got, ok := payloadFor(t, savedConfig(), func(v FieldValues, _ map[string]bool) { v["token"] = "   " }, "bee-bot")
	if _, present := got.Config["token"]; present || ok {
		t.Fatalf("got (%#v, %v), want the token key absent", got.Config, ok)
	}
}

func TestBuildSettingsPayloadSendsANewSecretTrimmed(t *testing.T) {
	got, ok := payloadFor(t, savedConfig(), func(v FieldValues, _ map[string]bool) { v["token"] = " new-token " }, "bee-bot")
	if !ok || !reflect.DeepEqual(got.Config, map[string]any{"token": "new-token"}) {
		t.Fatalf("got (%#v, %v)", got.Config, ok)
	}
}

func TestBuildSettingsPayloadClearsSavedOptionalValuesByType(t *testing.T) {
	got, ok := payloadFor(t, savedConfig(), func(v FieldValues, _ map[string]bool) {
		v["name"] = ""
		v["port"] = ""
		v["admins"] = "  "
	}, "bee-bot")
	want := map[string]any{"name": "", "port": nil, "admins": []string{}}
	if !ok || !reflect.DeepEqual(got.Config, want) {
		t.Fatalf("got (%#v, %v), want %#v", got.Config, ok, want)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"config":{"admins":[],"name":"","port":null}}` {
		t.Fatalf("json = %s", raw)
	}
}

func TestBuildSettingsPayloadIntClear(t *testing.T) {
	got, ok := payloadFor(t, savedConfig(), func(v FieldValues, _ map[string]bool) { v["port"] = " " }, "bee-bot")
	value, present := got.Config["port"]
	if !ok || !present || value != nil {
		t.Fatalf("got (%#v, %v), want port sent as nil", got.Config, ok)
	}
}

func TestBuildSettingsPayloadEmptyUnsavedIntIsNotSent(t *testing.T) {
	got, ok := payloadFor(t, map[string]any{}, func(v FieldValues, _ map[string]bool) {
		v["secret_path"] = "keep"
	}, "bee-bot")
	if _, present := got.Config["port"]; present {
		t.Fatalf("port must be left to the plugin default: %#v", got.Config)
	}
	if !ok {
		t.Fatal("secret_path is sent, so ok must be true")
	}
}

func TestBuildSettingsPayloadSendsChangedValuesWithTheirTypes(t *testing.T) {
	got, ok := payloadFor(t, savedConfig(), func(v FieldValues, b map[string]bool) {
		v["port"] = "9100"
		v["admins"] = "a\n b \n\nc"
		b["debug"] = false
	}, "bee-bot")
	want := map[string]any{"port": int64(9100), "admins": []string{"a", "b", "c"}, "debug": false}
	if !ok || !reflect.DeepEqual(got.Config, want) {
		t.Fatalf("got (%#v, %v), want %#v", got.Config, ok, want)
	}
}

func TestBuildSettingsPayloadIntComparesNumerically(t *testing.T) {
	got, ok := payloadFor(t, savedConfig(), func(v FieldValues, _ map[string]bool) { v["port"] = " 9000 " }, "bee-bot")
	if ok || len(got.Config) != 0 {
		t.Fatalf("an equal number must not be sent: %#v", got.Config)
	}
	got, ok = payloadFor(t, savedConfig(), func(v FieldValues, _ map[string]bool) { v["port"] = "-7" }, "bee-bot")
	if !ok || got.Config["port"] != int64(-7) {
		t.Fatalf("got (%#v, %v), want port -7", got.Config, ok)
	}
}

func TestBuildSettingsPayloadIntSendsUnsavedValue(t *testing.T) {
	got, ok := payloadFor(t, map[string]any{}, func(v FieldValues, _ map[string]bool) {
		v["port"] = "8080"
		v["secret_path"] = ""
	}, "bee-bot")
	if !ok || got.Config["port"] != int64(8080) {
		t.Fatalf("got (%#v, %v), want port 8080 sent even if it equals the default", got.Config, ok)
	}
}

func TestBuildSettingsPayloadListUnchangedIsNotSent(t *testing.T) {
	got, ok := payloadFor(t, savedConfig(), func(v FieldValues, _ map[string]bool) { v["admins"] = " a \n\n" }, "bee-bot")
	if ok || len(got.Config) != 0 {
		t.Fatalf("got (%#v, %v), want nothing", got.Config, ok)
	}
}

func TestBuildSettingsPayloadListUnsavedEmptyIsNotSent(t *testing.T) {
	got, _ := payloadFor(t, map[string]any{}, func(v FieldValues, _ map[string]bool) { v["admins"] = "\n \n" }, "bee-bot")
	if _, present := got.Config["admins"]; present {
		t.Fatalf("an empty unsaved list must be left alone: %#v", got.Config)
	}
}

func TestBuildSettingsPayloadLeavesUnsavedEmptyFieldsToPluginDefaults(t *testing.T) {
	got, ok := payloadFor(t, map[string]any{}, func(v FieldValues, _ map[string]bool) { v["secret_path"] = "keep" }, "bee-bot")
	if !ok || !reflect.DeepEqual(got.Config, map[string]any{"secret_path": "keep"}) {
		t.Fatalf("got (%#v, %v)", got.Config, ok)
	}
}

func TestBuildSettingsPayloadRandomDefault(t *testing.T) {
	got, ok := payloadFor(t, map[string]any{}, nil, "bee-bot")
	if !ok || !reflect.DeepEqual(got.Config, map[string]any{"secret_path": "0123456789abcdef"}) {
		t.Fatalf("got (%#v, %v), want the generated value sent", got.Config, ok)
	}
}

func TestBuildSettingsPayloadEmptiedRandomDefaultSendsAnExplicitEmptyString(t *testing.T) {
	got, ok := payloadFor(t, map[string]any{}, func(v FieldValues, _ map[string]bool) { v["secret_path"] = "" }, "bee-bot")
	if !ok || !reflect.DeepEqual(got.Config, map[string]any{"secret_path": ""}) {
		t.Fatalf("got (%#v, %v)", got.Config, ok)
	}
}

func TestBuildSettingsPayloadRandomDefaultKeyPresentWithNullIsNotGenerated(t *testing.T) {
	got, ok := payloadFor(t, map[string]any{"secret_path": nil}, nil, "bee-bot")
	if ok || len(got.Config) != 0 {
		t.Fatalf("got (%#v, %v), want nothing: the key exists, so no value is generated", got.Config, ok)
	}
}

func TestBuildSettingsPayloadBoolDiffersFromDefault(t *testing.T) {
	got, ok := payloadFor(t, map[string]any{}, func(v FieldValues, b map[string]bool) {
		v["secret_path"] = ""
		b["debug"] = false
	}, "bee-bot")
	if !ok || got.Config["debug"] != false {
		t.Fatalf("got (%#v, %v), want debug false sent", got.Config, ok)
	}
}

func TestBuildSettingsPayloadSavedStringChange(t *testing.T) {
	got, ok := payloadFor(t, savedConfig(), func(v FieldValues, _ map[string]bool) { v["name"] = " bee2 " }, "bee-bot")
	if !ok || !reflect.DeepEqual(got.Config, map[string]any{"name": " bee2 "}) {
		t.Fatalf("string values are sent untrimmed: %#v", got.Config)
	}
}

func TestBuildSettingsPayloadBotName(t *testing.T) {
	t.Run("unchanged", func(t *testing.T) {
		got, ok := payloadFor(t, savedConfig(), nil, "  bee-bot ")
		if ok || got.BotName != nil {
			t.Fatalf("got (%#v, %v), want BotName nil", got, ok)
		}
	})
	t.Run("changed", func(t *testing.T) {
		got, ok := payloadFor(t, savedConfig(), nil, " new-bot ")
		if !ok || got.BotName == nil || *got.BotName != "new-bot" {
			t.Fatalf("got (%#v, %v), want the trimmed bot name", got, ok)
		}
		if got.Config == nil || len(got.Config) != 0 {
			t.Fatalf("Config must be an empty non-nil map, got %#v", got.Config)
		}
	})
	t.Run("cleared", func(t *testing.T) {
		got, ok := payloadFor(t, savedConfig(), nil, "")
		if !ok || got.BotName == nil || *got.BotName != "" {
			t.Fatalf("got (%#v, %v), want an explicit empty bot name", got, ok)
		}
	})
	t.Run("changed along with config", func(t *testing.T) {
		got, ok := payloadFor(t, savedConfig(), func(v FieldValues, _ map[string]bool) { v["name"] = "x" }, "other")
		if !ok || got.BotName == nil || got.Config["name"] != "x" {
			t.Fatalf("got (%#v, %v)", got, ok)
		}
	})
}

func catalogEntry(mutate ...func(*api.CatalogPlugin)) api.CatalogPlugin {
	p := api.CatalogPlugin{
		Category:        "channel",
		ID:              "telegram",
		Package:         "@theopenbee/channel-telegram",
		Version:         "0.2.0",
		Official:        true,
		Name:            api.LocalizedText{"en": "Telegram", "zh": "Telegram"},
		Description:     api.LocalizedText{"en": "Bot API channel", "zh": "机器人渠道"},
		ProtocolVersion: 1,
		License:         "MIT",
		Repository:      "https://github.com/theopenbee/channel-telegram",
		Tags:            []string{"global"},
		Installable:     true,
	}
	for _, f := range mutate {
		f(&p)
	}
	return p
}

func catalogOf(plugins ...api.CatalogPlugin) api.PluginCatalog {
	return api.PluginCatalog{
		Tags: []api.CatalogTag{
			{ID: "global", Name: api.LocalizedText{"en": "Global", "zh": "全球"}},
			{ID: "china", Name: api.LocalizedText{"en": "China", "zh": "中国"}},
			{ID: "unused", Name: api.LocalizedText{"en": "Unused"}},
		},
		Plugins: plugins,
	}
}

func weixinEntry() api.CatalogPlugin {
	return catalogEntry(func(p *api.CatalogPlugin) {
		p.ID = "weixin"
		p.Package = "@theopenbee/channel-weixin"
		p.Name = api.LocalizedText{"en": "WeChat", "zh": "微信"}
		p.Description = api.LocalizedText{"zh": "扫码登录"}
		p.Tags = []string{"china"}
	})
}

func catalogIDs(ps []api.CatalogPlugin) []string {
	ids := make([]string, len(ps))
	for i, p := range ps {
		ids[i] = p.ID
	}
	return ids
}

func TestFilterCatalog(t *testing.T) {
	c := catalogOf(catalogEntry(), weixinEntry())
	tests := []struct {
		name  string
		tag   string
		query string
		want  []string
	}{
		{"no filter", "", "", []string{"telegram", "weixin"}},
		{"matches a localized name", "", "微信", []string{"weixin"}},
		{"matches a description", "", "bot api", []string{"telegram"}},
		{"matches a description in any language", "", "扫码", []string{"weixin"}},
		{"matches the id case-insensitively", "", "WEIX", []string{"weixin"}},
		{"trims the query", "", "  wechat  ", []string{"weixin"}},
		{"blank query matches everything", "", "   ", []string{"telegram", "weixin"}},
		{"filters by tag", "china", "", []string{"weixin"}},
		{"tag and query together", "global", "wechat", nil},
		{"tag nobody has", "unused", "", nil},
		{"no match", "", "zzz", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := catalogIDs(FilterCatalog(c, tc.tag, tc.query)); !slices.Equal(got, tc.want) && !(len(got) == 0 && len(tc.want) == 0) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUsedTags(t *testing.T) {
	c := catalogOf(catalogEntry(), weixinEntry())
	got := UsedTags(c)
	ids := make([]string, len(got))
	for i, tag := range got {
		ids[i] = tag.ID
	}
	if want := []string{"global", "china"}; !slices.Equal(ids, want) {
		t.Fatalf("got %v, want %v", ids, want)
	}
	if got[0].Name["zh"] != "全球" {
		t.Fatalf("tag names must be carried over: %v", got[0])
	}
	if got := UsedTags(catalogOf()); len(got) != 0 {
		t.Fatalf("a catalog without plugins uses no tags, got %v", got)
	}
}

func TestCatalogAction(t *testing.T) {
	tests := []struct {
		name       string
		plugin     api.CatalogPlugin
		installing bool
		want       CardAction
	}{
		{"install", catalogEntry(), false, ActionInstall},
		{"installing", catalogEntry(), true, ActionInstalling},
		{"installing wins over installed", catalogEntry(func(p *api.CatalogPlugin) { p.Installed = true }), true, ActionInstalling},
		{"incompatible", catalogEntry(func(p *api.CatalogPlugin) { p.Installable = false }), false, ActionIncompatible},
		{"configure", catalogEntry(func(p *api.CatalogPlugin) { p.Installed = true }), false, ActionConfigure},
		{
			"update",
			catalogEntry(func(p *api.CatalogPlugin) { p.Installed = true; p.UpdateAvailable = true }),
			false,
			ActionUpdate,
		},
		{
			"update that is not installable falls back to configure",
			catalogEntry(func(p *api.CatalogPlugin) { p.Installed = true; p.UpdateAvailable = true; p.Installable = false }),
			false,
			ActionConfigure,
		},
		{
			"installed but not installable still configures",
			catalogEntry(func(p *api.CatalogPlugin) { p.Installed = true; p.Installable = false }),
			false,
			ActionConfigure,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CatalogAction(tc.plugin, tc.installing); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestLocalize(t *testing.T) {
	tests := []struct {
		name string
		text api.LocalizedText
		lang string
		want string
	}{
		{"exact language", api.LocalizedText{"en": "Hi", "zh": "你好"}, "zh", "你好"},
		{"base language", api.LocalizedText{"en": "Hi", "zh": "你好"}, "zh-CN", "你好"},
		{"exact region beats base", api.LocalizedText{"zh": "简", "zh-TW": "繁"}, "zh-TW", "繁"},
		{"falls back to English", api.LocalizedText{"en": "Hi"}, "zh", "Hi"},
		{"falls back to the first value", api.LocalizedText{"fr": "Salut"}, "zh", "Salut"},
		{"first value is the first key in order", api.LocalizedText{"fr": "Salut", "de": "Hallo"}, "zh", "Hallo"},
		{"same base with another region", api.LocalizedText{"en": "Hi", "zh-TW": "繁"}, "zh-CN", "繁"},
		{"nil", nil, "en", ""},
		{"empty", api.LocalizedText{}, "en", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := localize(tc.text, tc.lang); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPluginName(t *testing.T) {
	catalog := catalogOf(catalogEntry(), weixinEntry())
	installed := []api.PluginStatus{
		{ID: "line", Name: api.LocalizedText{"en": "LINE"}},
		{ID: "telegram", Name: api.LocalizedText{"en": "Installed Telegram"}},
	}
	tests := []struct {
		name      string
		id        string
		catalog   *api.PluginCatalog
		installed []api.PluginStatus
		lang      string
		want      string
	}{
		{"catalog name first", "weixin", &catalog, installed, "zh", "微信"},
		{"catalog name beats installed name", "telegram", &catalog, installed, "en", "Telegram"},
		{"installed name", "line", &catalog, installed, "en", "LINE"},
		{"no catalog", "line", nil, installed, "en", "LINE"},
		{"unknown id", "slack", &catalog, installed, "en", "slack"},
		{"empty id", "", &catalog, installed, "en", ""},
		{"no data at all", "slack", nil, nil, "en", "slack"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PluginName(tc.id, tc.catalog, tc.installed, tc.lang); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPluginNameCatalogEntryWithoutNameFallsBackToID(t *testing.T) {
	catalog := catalogOf(catalogEntry(func(p *api.CatalogPlugin) { p.Name = nil }))
	installed := []api.PluginStatus{{ID: "telegram", Name: api.LocalizedText{"en": "Installed"}}}
	if got := PluginName("telegram", &catalog, installed, "en"); got != "telegram" {
		t.Fatalf("got %q, want the id", got)
	}
}

func TestPluginNeedsPolling(t *testing.T) {
	status := func(states ...string) []api.PluginStatus {
		out := make([]api.PluginStatus, len(states))
		for i, s := range states {
			out[i] = api.PluginStatus{ID: s, State: s}
		}
		return out
	}
	tests := []struct {
		name string
		in   []api.PluginStatus
		want bool
	}{
		{"starting", status("running", "starting"), true},
		{"retrying", status("retrying"), true},
		{"settled", status("running", "failed", "not_configured", "disabled"), false},
		{"none", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PluginNeedsPolling(tc.in); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOperationsPending(t *testing.T) {
	ops := func(statuses ...string) []api.PluginOperation {
		out := make([]api.PluginOperation, len(statuses))
		for i, s := range statuses {
			out[i] = api.PluginOperation{ID: s, Status: s}
		}
		return out
	}
	tests := []struct {
		name string
		in   []api.PluginOperation
		want bool
	}{
		{"queued", ops("succeeded", "queued"), true},
		{"running", ops("running"), true},
		{"finished", ops("succeeded", "failed"), false},
		{"none", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := OperationsPending(tc.in); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSpecPackage(t *testing.T) {
	tests := []struct {
		spec string
		want string
	}{
		{"@theopenbee/channel-telegram@0.2.0", "@theopenbee/channel-telegram"},
		{"@theopenbee/channel-telegram", "@theopenbee/channel-telegram"},
		{"left-pad@1.0.0", "left-pad"},
		{"left-pad", "left-pad"},
		{"/tmp/plugin", "/tmp/plugin"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := SpecPackage(tc.spec); got != tc.want {
			t.Errorf("SpecPackage(%q) = %q, want %q", tc.spec, got, tc.want)
		}
	}
}

func TestPendingInstalls(t *testing.T) {
	op := func(mutate func(*api.PluginOperation)) api.PluginOperation {
		o := api.PluginOperation{
			ID:        "op",
			Kind:      "install",
			Spec:      "@theopenbee/channel-telegram@0.2.0",
			Status:    "running",
			CreatedAt: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
		}
		mutate(&o)
		return o
	}
	got := PendingInstalls([]api.PluginOperation{
		op(func(o *api.PluginOperation) { o.ID = "a" }),
		op(func(o *api.PluginOperation) { o.ID = "dup" }),
		op(func(o *api.PluginOperation) { o.ID = "b"; o.Spec = "@theopenbee/channel-weixin"; o.Status = "queued" }),
		op(func(o *api.PluginOperation) {
			o.ID = "c"
			o.Spec = "@theopenbee/channel-slack@1.0.0"
			o.Status = "succeeded"
		}),
		op(func(o *api.PluginOperation) { o.ID = "d"; o.Kind = "uninstall"; o.Spec = ""; o.PluginID = "line" }),
		op(func(o *api.PluginOperation) { o.ID = "e"; o.Spec = "" }),
	})
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if want := []string{"@theopenbee/channel-telegram", "@theopenbee/channel-weixin"}; !slices.Equal(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	if got["@theopenbee/channel-telegram"].ID != "a" {
		t.Fatalf("the first pending op per package wins, got %q", got["@theopenbee/channel-telegram"].ID)
	}
}

func TestRandomHex16(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9a-f]{16}$`)
	first, second := RandomHex16(), RandomHex16()
	if !pattern.MatchString(first) || !pattern.MatchString(second) {
		t.Fatalf("got %q and %q, want 16 lowercase hex characters", first, second)
	}
	if first == second {
		t.Fatalf("two calls returned the same value %q", first)
	}
}

func TestPluginType(t *testing.T) {
	tests := map[string]string{"": PluginChannel, "channel": PluginChannel, "engine": PluginEngine, "tool": "tool"}
	for in, want := range tests {
		if got := PluginType(in); got != want {
			t.Fatalf("PluginType(%q) = %q, want %q", in, got, want)
		}
	}
	if IsEngine("") || IsEngine("channel") || !IsEngine("engine") {
		t.Fatal("IsEngine must be true for engine only")
	}
}

func TestIsActiveState(t *testing.T) {
	active := []string{"running", "ready"}
	idle := []string{"", "starting", "retrying", "failed", "not_configured", "disabled"}
	for _, s := range active {
		if !IsActiveState(s) {
			t.Fatalf("%q must count as active", s)
		}
	}
	for _, s := range idle {
		if IsActiveState(s) {
			t.Fatalf("%q must not count as active", s)
		}
	}
}

func TestCountActive(t *testing.T) {
	list := []api.PluginStatus{
		{ID: "telegram", State: "running"},
		{ID: "claude", Type: "engine", State: "ready"},
		{ID: "codex", Type: "engine", State: "failed"},
		{ID: "discord", State: "disabled"},
	}
	if got := CountActive(list); got != 2 {
		t.Fatalf("CountActive = %d, want 2", got)
	}
	if got := CountActive(nil); got != 0 {
		t.Fatalf("CountActive(nil) = %d, want 0", got)
	}
}

func TestGroupByType(t *testing.T) {
	typeOf := func(s api.PluginStatus) string { return s.Type }
	list := []api.PluginStatus{
		{ID: "claude", Type: "engine"},
		{ID: "telegram"},
		{ID: "hook", Type: "tool"},
		{ID: "discord", Type: "channel"},
		{ID: "pi", Type: "engine"},
	}
	groups := GroupByType(list, typeOf)
	var got []string
	for _, g := range groups {
		ids := make([]string, len(g.Items))
		for i, item := range g.Items {
			ids[i] = item.ID
		}
		got = append(got, g.Type+":"+strings.Join(ids, ","))
	}
	want := []string{"channel:telegram,discord", "engine:claude,pi", "tool:hook"}
	if !slices.Equal(got, want) {
		t.Fatalf("groups = %v, want %v", got, want)
	}
	if groups := GroupByType([]api.PluginStatus{{ID: "pi", Type: "engine"}}, typeOf); len(groups) != 1 || groups[0].Type != "engine" {
		t.Fatalf("a single engine must not invent an empty channel group: %+v", groups)
	}
	if groups := GroupByType(nil, typeOf); len(groups) != 0 {
		t.Fatalf("no items must give no groups, got %+v", groups)
	}
}

func TestCategoryName(t *testing.T) {
	categories := []api.CatalogTag{
		{ID: "channel", Name: api.LocalizedText{"en": "Channels", "zh": "渠道"}},
		{ID: "engine", Name: api.LocalizedText{"en": "Engines"}},
	}
	tests := []struct{ typ, lang, want string }{
		{"channel", "zh", "渠道"},
		{"engine", "zh", "Engines"},
		{"engine", "en", "Engines"},
		{"tool", "en", ""},
	}
	for _, tc := range tests {
		if got := CategoryName(categories, tc.typ, tc.lang); got != tc.want {
			t.Fatalf("CategoryName(%q, %q) = %q, want %q", tc.typ, tc.lang, got, tc.want)
		}
	}
	if got := CategoryName(nil, "engine", "en"); got != "" {
		t.Fatalf("CategoryName without categories = %q, want empty", got)
	}
}

func TestBuildSettingsPayloadEngineNeverSendsBotName(t *testing.T) {
	d := detailOf([]api.PluginConfigField{{Key: "args", Type: "string"}}, map[string]any{}, "")
	d.Type = "engine"
	values, bools := InitialFieldValues(d, fixedRandom)
	got, ok := BuildSettingsPayload(d, values, bools, "typed-by-mistake")
	if ok || got.BotName != nil || len(got.Config) != 0 {
		t.Fatalf("got (%#v, %v), want nothing to send for an engine", got, ok)
	}
	values["args"] = "--model opus"
	got, ok = BuildSettingsPayload(d, values, bools, "typed-by-mistake")
	if !ok || got.BotName != nil || got.Config["args"] != "--model opus" {
		t.Fatalf("got (%#v, %v), want the config without a bot name", got, ok)
	}
	d.Type = "channel"
	got, ok = BuildSettingsPayload(d, values, bools, "typed-by-mistake")
	if !ok || got.BotName == nil || *got.BotName != "typed-by-mistake" {
		t.Fatalf("got (%#v, %v), a channel keeps sending its bot name", got, ok)
	}
}
