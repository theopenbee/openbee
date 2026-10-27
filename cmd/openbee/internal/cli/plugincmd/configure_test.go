package plugincmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/plugin"
)

func tgFields() []plugin.ConfigField {
	return []plugin.ConfigField{
		{Key: "token", Type: plugin.FieldSecret, Required: true, Label: plugin.LocalizedText{"en": "Bot token"}},
		{Key: "auth_code", Type: plugin.FieldString, RandomDefault: true},
		{Key: "max_media_size", Type: plugin.FieldInt, Default: 10.0},
		{Key: "debug", Type: plugin.FieldBool},
		{Key: "allowed", Type: plugin.FieldList},
	}
}

func tgDetail(config map[string]any) Detail {
	return Detail{Plugin: Plugin{ID: "telegram", State: "disabled", Path: "/plugins/telegram"}, Fields: tgFields(), Config: config}
}

func noSetup(string) (map[string]any, error) { return nil, errors.New("unexpected setup") }

func TestParseSets(t *testing.T) {
	got, err := parseSets(tgFields(), []string{"token=abc=def", "max_media_size=42", "debug=true", "allowed=a, b ,c", "auth_code=", "extra=1"})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"token": "abc=def", "max_media_size": 42, "debug": true,
		"allowed": []string{"a", "b", "c"}, "auth_code": "", "extra": "1",
	}, got)

	_, err = parseSets(tgFields(), []string{"novalue"})
	assert.Error(t, err)
	_, err = parseSets(tgFields(), []string{"max_media_size=big"})
	assert.Error(t, err)
	_, err = parseSets(tgFields(), []string{"debug=maybe"})
	assert.Error(t, err)
}

func TestCollectSettings_KeepsSecretAndClearedRandomDefaultIsEmpty(t *testing.T) {
	p := &fakePrompter{interactive: true, answers: []any{"", "-", "10", false, "", "TBot"}}
	bot, cfg, err := collectSettings(tgDetail(map[string]any{"token": "****abcd"}), p, noSetup, &bytes.Buffer{})
	require.NoError(t, err)
	assert.NotContains(t, cfg, "token")
	assert.Equal(t, "", cfg["auth_code"])
	var authDefault string
	for msg, def := range p.defaults {
		if strings.HasPrefix(msg, "auth_code") {
			authDefault = def
		}
	}
	assert.Regexp(t, `^[0-9a-f]{16}$`, authDefault)
	assert.NotContains(t, cfg, "allowed")
	assert.Equal(t, 10, cfg["max_media_size"])
	assert.Equal(t, false, cfg["debug"])
	require.NotNil(t, bot)
	assert.Equal(t, "TBot", *bot)
}

func TestCollectSettings_RequiredSecretAskedAgain(t *testing.T) {
	p := &fakePrompter{interactive: true, answers: []any{"", "tok", "abc", "", false, "a, b", ""}}
	bot, cfg, err := collectSettings(tgDetail(nil), p, noSetup, &bytes.Buffer{})
	require.NoError(t, err)
	assert.Equal(t, "tok", cfg["token"])
	assert.Equal(t, "abc", cfg["auth_code"])
	assert.NotContains(t, cfg, "max_media_size")
	assert.Equal(t, []string{"a", "b"}, cfg["allowed"])
	assert.Nil(t, bot)
}

func TestCollectSettings_SetupFillsFields(t *testing.T) {
	d := tgDetail(nil)
	d.HasSetup = true
	d.SetupLabel = plugin.LocalizedText{"en": "QR login"}
	var setupPath string
	setup := func(path string) (map[string]any, error) {
		setupPath = path
		return map[string]any{"token": "from-qr"}, nil
	}
	p := &fakePrompter{interactive: true, answers: []any{true, "", "", false, "", ""}}
	_, cfg, err := collectSettings(d, p, setup, &bytes.Buffer{})
	require.NoError(t, err)
	assert.Equal(t, "/plugins/telegram", setupPath)
	assert.Equal(t, "from-qr", cfg["token"])
	assert.NotContains(t, p.asked, "password:Bot token")
}

func TestCollectSettings_SetupFailureFallsBack(t *testing.T) {
	d := tgDetail(nil)
	d.HasSetup = true
	failing := func(string) (map[string]any, error) { return nil, errors.New("qr expired") }
	p := &fakePrompter{interactive: true, answers: []any{true, "manual", "", "", false, "", ""}}
	var out bytes.Buffer
	_, cfg, err := collectSettings(d, p, failing, &out)
	require.NoError(t, err)
	assert.Equal(t, "manual", cfg["token"])
	assert.Contains(t, out.String(), "qr expired")
}

func TestConfigure_NonTTYWithoutFlagsErrors(t *testing.T) {
	api := newFakeAPI()
	api.addPlugin(tgDetail(nil))
	_, err := runCmd(t, api, &fakePrompter{}, "configure", "telegram")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--set")
	assert.Empty(t, api.actions)
}

func TestConfigure_SetFlagsEnableAndWait(t *testing.T) {
	api := newFakeAPI()
	api.addPlugin(tgDetail(nil))
	api.states["telegram"] = []string{"starting", "running"}
	out, err := runCmd(t, api, &fakePrompter{}, "configure", "telegram", "--set", "token=abc", "--bot-name", "TBot", "--enable")
	require.NoError(t, err)
	assert.Equal(t, []string{"settings:telegram", "enable:telegram"}, api.actions)
	assert.Equal(t, map[string]any{"token": "abc"}, api.settings["telegram"])
	require.NotNil(t, api.botNames["telegram"])
	assert.Equal(t, "TBot", *api.botNames["telegram"])
	assert.Contains(t, out, "telegram is running")
}

func TestConfigure_FailedStateIsAnError(t *testing.T) {
	api := newFakeAPI()
	api.addPlugin(tgDetail(nil))
	api.states["telegram"] = []string{"starting", "failed"}
	_, err := runCmd(t, api, &fakePrompter{}, "configure", "telegram", "--set", "token=bad", "--enable")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad token")
}

func TestConfigure_InteractiveEnablesWhenConfirmed(t *testing.T) {
	api := newFakeAPI()
	api.addPlugin(tgDetail(map[string]any{"token": "****abcd"}))
	api.states["telegram"] = []string{"running"}
	p := &fakePrompter{interactive: true, answers: []any{"", "", "10", false, "", "", true}}
	out, err := runCmd(t, api, p, "configure", "telegram")
	require.NoError(t, err)
	assert.Equal(t, []string{"settings:telegram", "enable:telegram"}, api.actions)
	assert.Contains(t, out, "telegram is running")
}

func TestConfigure_EnabledPluginRestartsAndWaits(t *testing.T) {
	api := newFakeAPI()
	d := tgDetail(map[string]any{"token": "****abcd"})
	d.Enabled, d.State = true, "running"
	api.addPlugin(d)
	api.states["telegram"] = []string{"starting", "running"}
	out, err := runCmd(t, api, &fakePrompter{}, "configure", "telegram", "--set", "debug=true")
	require.NoError(t, err)
	assert.Equal(t, []string{"settings:telegram"}, api.actions)
	assert.Contains(t, out, "telegram is running")
}

func TestWaitRunning_StillStarting(t *testing.T) {
	api := newFakeAPI()
	api.addPlugin(tgDetail(nil))
	api.states["telegram"] = []string{"starting"}
	var out bytes.Buffer
	err := waitRunning(context.Background(), api.client(t), "telegram", &out, time.Millisecond, 20*time.Millisecond)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "still starting")
}

func TestCollectSettings_ExplicitEmptyRandomDefaultIsNotRefilled(t *testing.T) {
	p := &fakePrompter{interactive: true, answers: []any{"", "", "10", false, "", ""}}
	_, cfg, err := collectSettings(tgDetail(map[string]any{"token": "****abcd", "auth_code": ""}), p, noSetup, &bytes.Buffer{})
	require.NoError(t, err)
	assert.Equal(t, "", cfg["auth_code"])
}

func TestCollectSettings_EnterAcceptsPrefilledRandomDefault(t *testing.T) {
	p := &fakePrompter{interactive: true, answers: []any{"tok", "", "", false, "", ""}}
	_, cfg, err := collectSettings(tgDetail(nil), p, noSetup, &bytes.Buffer{})
	require.NoError(t, err)
	assert.Regexp(t, `^[0-9a-f]{16}$`, cfg["auth_code"])
	assert.NotContains(t, cfg, "max_media_size")
}

func TestCollectSettings_DashClearsCurrentValues(t *testing.T) {
	d := tgDetail(map[string]any{"token": "****abcd", "auth_code": "abc", "max_media_size": 20.0, "allowed": []any{"a"}})
	d.BotName = "TBot"
	p := &fakePrompter{interactive: true, answers: []any{"", "-", "-", false, "-", "-"}}
	bot, cfg, err := collectSettings(d, p, noSetup, &bytes.Buffer{})
	require.NoError(t, err)
	assert.Equal(t, "", cfg["auth_code"])
	assert.Equal(t, []string{}, cfg["allowed"])
	v, present := cfg["max_media_size"]
	assert.True(t, present)
	assert.Nil(t, v)
	require.NotNil(t, bot)
	assert.Equal(t, "", *bot)
}

func TestCollectSettings_ShowsKeepHintAndHelp(t *testing.T) {
	d := tgDetail(map[string]any{"token": "****abcd"})
	d.Fields[0].Help = plugin.LocalizedText{"en": "Get it from @BotFather"}
	p := &fakePrompter{interactive: true, answers: []any{"", "", "", false, "", ""}}
	var out bytes.Buffer
	_, _, err := collectSettings(d, p, noSetup, &out)
	require.NoError(t, err)
	assert.Contains(t, p.asked[0], "****abcd")
	assert.Contains(t, out.String(), "Get it from @BotFather")
}
