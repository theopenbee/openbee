package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/egoist/mygo/i18n"
)

func render(t *testing.T, icu, locale string, args map[string]any) string {
	t.Helper()
	msg, err := i18n.ParseMessage(icu)
	if err != nil {
		t.Fatalf("ParseMessage(%q): %v", icu, err)
	}
	return msg.Format(locale, args)
}

func TestFlatKey(t *testing.T) {
	cases := []struct {
		path []string
		want string
	}{
		{[]string{"chat", "messagePlaceholder"}, "chatMessagePlaceholder"},
		{[]string{"errors", "worker_not_found"}, "errorsWorkerNotFound"},
		{[]string{"arc", "userMenu", "themes", "light"}, "arcUserMenuThemesLight"},
		{[]string{"plugins", "installDialog", "placeholder"}, "pluginsInstallDialogPlaceholder"},
		{[]string{"a", "b-c d", "e2f"}, "aBCDE2f"},
	}
	for _, c := range cases {
		if got := FlatKey(c.path); got != c.want {
			t.Errorf("FlatKey(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestToICU(t *testing.T) {
	if got := ToICU("Message {{name}}"); got != "Message {name}" {
		t.Errorf("ToICU interpolation = %q", got)
	}
	if got := ToICU("Message {{ name }}"); got != "Message {name}" {
		t.Errorf("ToICU spaced interpolation = %q", got)
	}
	if got := ToICU("This worker's chat"); got != "This worker's chat" {
		t.Errorf("ToICU apostrophe = %q", got)
	}
	if got := ToICU("Use {braces}"); got == "Use {braces}" {
		t.Errorf("ToICU left a literal brace unescaped: %q", got)
	}

	cases := []struct {
		src  string
		args map[string]any
		want string
	}{
		{"This worker's chat", nil, "This worker's chat"},
		{"Use {braces} here", nil, "Use {braces} here"},
		{"{", nil, "{"},
		{"}", nil, "}"},
		{"}{", nil, "}{"},
		{"{}", nil, "{}"},
		{"{'}", nil, "{'}"},
		{"'{", nil, "'{"},
		{"''", nil, "''"},
		{"'", nil, "'"},
		{"a'", nil, "a'"},
		{"'a", nil, "'a"},
		{"{'", nil, "{'"},
		{"{''}", nil, "{''}"},
		{"100% #1", nil, "100% #1"},
		{"{{name}}'s files", map[string]any{"name": "Ada"}, "Ada's files"},
		{"it's {{name}}'", map[string]any{"name": "Ada"}, "it's Ada'"},
		{"'{{name}}'", map[string]any{"name": "Ada"}, "'Ada'"},
		{"Couldn't reach {{url}}.", map[string]any{"url": "http://x"}, "Couldn't reach http://x."},
		{"{{a}}{{b}}", map[string]any{"a": "1", "b": "2"}, "12"},
		{"{{a}} {braces}", map[string]any{"a": "1"}, "1 {braces}"},
		{"[Delivery]\n- Don't {break} it's\n", nil, "[Delivery]\n- Don't {break} it's\n"},
	}
	for _, c := range cases {
		icu := ToICU(c.src)
		if got := render(t, icu, "en", c.args); got != c.want {
			t.Errorf("ToICU(%q) = %q renders %q, want %q", c.src, icu, got, c.want)
		}
	}
}

func TestPluralMerge(t *testing.T) {
	en := map[string]any{"chat": map[string]any{
		"uploadError_one":   "{{count}} file failed to upload. Try again.",
		"uploadError_other": "{{count}} files failed to upload. Try again.",
	}}
	msgs, placeholders, err := Convert(en, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "{count, plural, one {# file failed to upload. Try again.} other {# files failed to upload. Try again.}}"
	if msgs["chatUploadError"] != want {
		t.Errorf("en message = %q, want %q", msgs["chatUploadError"], want)
	}
	if len(msgs) != 1 {
		t.Errorf("messages = %v, want only chatUploadError", msgs)
	}
	if got := placeholders["chatUploadError"]; !reflect.DeepEqual(got, []string{"count"}) {
		t.Errorf("placeholders = %v", got)
	}
	if got := render(t, msgs["chatUploadError"], "en", map[string]any{"count": 1}); got != "1 file failed to upload. Try again." {
		t.Errorf("en one = %q", got)
	}
	if got := render(t, msgs["chatUploadError"], "en", map[string]any{"count": 3}); got != "3 files failed to upload. Try again." {
		t.Errorf("en other = %q", got)
	}

	var arb map[string]any
	if err := json.Unmarshal(ARB("en", msgs, placeholders), &arb); err != nil {
		t.Fatal(err)
	}
	count := arb["@chatUploadError"].(map[string]any)["placeholders"].(map[string]any)["count"].(map[string]any)
	if count["type"] != "int" {
		t.Errorf("count placeholder = %v, want type int", count)
	}

	zh := map[string]any{"chat": map[string]any{
		"uploadError_other": "{{count}} 个文件上传失败，请重试",
	}}
	zhMsgs, _, err := Convert(zh, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := "{count, plural, other {# 个文件上传失败，请重试}}"; zhMsgs["chatUploadError"] != want {
		t.Errorf("zh message = %q, want %q", zhMsgs["chatUploadError"], want)
	}
}

func TestPluralKeepsOtherPlaceholders(t *testing.T) {
	web := map[string]any{"plugins": map[string]any{
		"installedSummary_one":   "{{count}} plugin, {{running}} running",
		"installedSummary_other": "{{count}} plugins, {{running}} running",
	}}
	msgs, placeholders, err := Convert(web, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := placeholders["pluginsInstalledSummary"]; !reflect.DeepEqual(got, []string{"count", "running"}) {
		t.Errorf("placeholders = %v", got)
	}
	if got := render(t, msgs["pluginsInstalledSummary"], "en", map[string]any{"count": 1, "running": "0"}); got != "1 plugin, 0 running" {
		t.Errorf("rendered = %q", got)
	}
	var arb map[string]any
	if err := json.Unmarshal(ARB("en", msgs, placeholders), &arb); err != nil {
		t.Fatal(err)
	}
	ph := arb["@pluginsInstalledSummary"].(map[string]any)["placeholders"].(map[string]any)
	if ph["count"].(map[string]any)["type"] != "int" || ph["running"].(map[string]any)["type"] != "String" {
		t.Errorf("placeholder types = %v", ph)
	}
}

func TestPluralQuotesHash(t *testing.T) {
	web := map[string]any{"x": map[string]any{
		"item_one":   "{{count}} item #1",
		"item_other": "{{count}} items #1",
	}}
	msgs, _, err := Convert(web, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := render(t, msgs["xItem"], "en", map[string]any{"count": 2}); got != "2 items #1" {
		t.Errorf("rendered = %q", got)
	}
}

func TestConvertFlattensAndOverlays(t *testing.T) {
	web := map[string]any{
		"common": map[string]any{"cancel": "Cancel"},
		"chat":   map[string]any{"messagePlaceholder": "Message {{name}}"},
	}
	msgs, placeholders, err := Convert(web, map[string]string{"connectTitle": "Connect to OpenBee", "downloadSaved": "Saved {name}"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"commonCancel":           "Cancel",
		"chatMessagePlaceholder": "Message {name}",
		"connectTitle":           "Connect to OpenBee",
		"downloadSaved":          "Saved {name}",
	}
	if !reflect.DeepEqual(msgs, want) {
		t.Errorf("messages = %v, want %v", msgs, want)
	}
	if got := placeholders["downloadSaved"]; !reflect.DeepEqual(got, []string{"name"}) {
		t.Errorf("overlay placeholders = %v", got)
	}
	if _, ok := placeholders["commonCancel"]; ok {
		t.Errorf("commonCancel has placeholders: %v", placeholders["commonCancel"])
	}
}

func TestConvertRejectsCollision(t *testing.T) {
	web := map[string]any{"a": map[string]any{"bC": "x", "b_c": "y"}}
	_, _, err := Convert(web, nil)
	if err == nil {
		t.Fatal("expected a collision error")
	}
	for _, name := range []string{"a.bC", "a.b_c", "aBC"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %q", err, name)
		}
	}
}

func TestConvertRejectsOverlayCollision(t *testing.T) {
	web := map[string]any{"common": map[string]any{"cancel": "Cancel"}}
	_, _, err := Convert(web, map[string]string{"commonCancel": "Stop"})
	if err == nil || !strings.Contains(err.Error(), "commonCancel") {
		t.Errorf("error = %v, want one naming commonCancel", err)
	}
}

func TestConvertRejectsPluralMixedWithPlainKey(t *testing.T) {
	web := map[string]any{"x": map[string]any{"item": "plain", "item_other": "{{count}} items"}}
	_, _, err := Convert(web, nil)
	if err == nil || !strings.Contains(err.Error(), "x.item") {
		t.Errorf("error = %v, want one naming x.item", err)
	}
}

func TestConvertRejectsBadInput(t *testing.T) {
	cases := map[string]map[string]any{
		"format spec":       {"x": map[string]any{"a": "{{count, number}}"}},
		"unclosed":          {"x": map[string]any{"a": "Hello {{name"}},
		"non string":        {"x": map[string]any{"a": 3.0}},
		"array":             {"x": map[string]any{"a": []any{"b"}}},
		"one without other": {"x": map[string]any{"a_one": "{{count}} a"}},
	}
	for name, web := range cases {
		if _, _, err := Convert(web, nil); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, _, err := Convert(map[string]any{}, map[string]string{"bad.key": "x"}); err == nil {
		t.Error("overlay key with a dot: expected an error")
	}
	if _, _, err := Convert(map[string]any{}, map[string]string{"badMessage": "{broken"}); err == nil {
		t.Error("overlay with invalid ICU: expected an error")
	}
}

func TestARBLayout(t *testing.T) {
	msgs := map[string]string{"zeta": "Z", "alpha": "Saved {name}"}
	placeholders := map[string][]string{"alpha": {"name"}}
	got := string(ARB("en", msgs, placeholders))
	want := `{
  "@@locale": "en",
  "alpha": "Saved {name}",
  "@alpha": { "placeholders": { "name": { "type": "String" } } },
  "zeta": "Z"
}
`
	if got != want {
		t.Errorf("ARB =\n%s\nwant\n%s", got, want)
	}
}

func TestErrorsSource(t *testing.T) {
	web := map[string]any{"errors": map[string]any{
		"worker_not_found":  "Worker not found",
		"env_invalid_scope": "Invalid scope {{scope}}",
	}}
	_, placeholders, err := Convert(web, nil)
	if err != nil {
		t.Fatal(err)
	}
	src, err := ErrorsSource(web, placeholders)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"package l10n",
		`case "worker_not_found":`,
		"return locales.ErrorsWorkerNotFound(), true",
		`case "env_invalid_scope":`,
		`locales.ErrorsEnvInvalidScope(param(params, "scope")), true`,
	} {
		if !strings.Contains(string(src), want) {
			t.Errorf("errors source lacks %q:\n%s", want, src)
		}
	}
	if strings.Index(string(src), "env_invalid_scope") > strings.Index(string(src), "worker_not_found") {
		t.Error("cases are not sorted by code")
	}

	countWeb := map[string]any{"errors": map[string]any{"total": "{{count}} errors"}}
	_, countPlaceholders, err := Convert(countWeb, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ErrorsSource(countWeb, countPlaceholders); err == nil {
		t.Error("expected an error for a non-string placeholder in errors.*")
	}
}

func TestCatalogUpToDate(t *testing.T) {
	out, err := build(filepath.FromSlash("../../../web/src/locales"))
	if err != nil {
		t.Fatal(err)
	}
	const hint = "is stale: run `go run ./tools/webcatalog -web ../web/src/locales -out locales -errors internal/l10n/errors_gen.go` in desktop/"
	for _, lang := range languages {
		path := filepath.Join("..", "..", "locales", lang+".json")
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v (run `go run ./tools/webcatalog ...`)", path, err)
		}
		if !bytes.Equal(got, out.Locales[lang]) {
			t.Errorf("%s %s", path, hint)
		}
	}
	path := filepath.Join("..", "..", "internal", "l10n", "errors_gen.go")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v (run `go run ./tools/webcatalog ...`)", path, err)
	}
	if !bytes.Equal(got, out.Errors) {
		t.Errorf("%s %s", path, hint)
	}
}

func TestBuiltCatalogsAgree(t *testing.T) {
	out, err := build(filepath.FromSlash("../../../web/src/locales"))
	if err != nil {
		t.Fatal(err)
	}
	var en, zh map[string]any
	if err := json.Unmarshal(out.Locales["en"], &en); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Locales["zh"], &zh); err != nil {
		t.Fatal(err)
	}
	for key := range en {
		if _, ok := zh[key]; !ok {
			t.Errorf("zh lacks %q", key)
		}
	}
	for key := range zh {
		if _, ok := en[key]; !ok {
			t.Errorf("en lacks %q", key)
		}
	}
	for _, key := range []string{"connectTitle", "downloadSaved", "chatUploadError", "errorsWorkerNotFound"} {
		if _, ok := en[key]; !ok {
			t.Errorf("en lacks %q", key)
		}
	}
}

func TestDeclareExtraPlaceholders(t *testing.T) {
	source := map[string][]string{"countdown": {"pending"}, "same": {"a"}}
	other := map[string][]string{"countdown": {"confirm"}, "same": {"a"}, "onlyOther": {"x"}}
	Declare(source, other)
	if got := source["countdown"]; !reflect.DeepEqual(got, []string{"pending", "confirm"}) {
		t.Errorf("countdown = %v", got)
	}
	if got := source["same"]; !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("same = %v", got)
	}
}

func TestWebMessagesRenderUnchanged(t *testing.T) {
	interpolation := regexp.MustCompile(`\{\{(\w+)\}\}`)
	for _, lang := range languages {
		web, err := readJSON[map[string]any](filepath.Join("..", "..", "..", "web", "src", "locales", lang+".json"))
		if err != nil {
			t.Fatal(err)
		}
		msgs, _, err := Convert(web, nil)
		if err != nil {
			t.Fatal(err)
		}
		checked := 0
		var visit func(path []string, node map[string]any)
		visit = func(path []string, node map[string]any) {
			for key, value := range node {
				next := append(append([]string(nil), path...), key)
				if child, ok := value.(map[string]any); ok {
					visit(next, child)
					continue
				}
				if strings.HasSuffix(key, "_one") || strings.HasSuffix(key, "_other") {
					continue
				}
				source := value.(string)
				args := map[string]any{}
				for _, match := range interpolation.FindAllStringSubmatch(source, -1) {
					if match[1] == "count" {
						args["count"] = 7
					} else {
						args[match[1]] = "<" + match[1] + ">"
					}
				}
				want := interpolation.ReplaceAllStringFunc(source, func(m string) string {
					name := interpolation.FindStringSubmatch(m)[1]
					if name == "count" {
						return "7"
					}
					return "<" + name + ">"
				})
				if got := render(t, msgs[FlatKey(next)], lang, args); got != want {
					t.Errorf("%s %s renders %q, want %q", lang, strings.Join(next, "."), got, want)
				}
				checked++
			}
		}
		visit(nil, web)
		if checked < 400 {
			t.Errorf("%s: only %d messages checked", lang, checked)
		}
	}
}
