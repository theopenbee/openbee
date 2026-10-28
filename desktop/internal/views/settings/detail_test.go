package settings_test

import (
	"encoding/json"
	"maps"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/views/settings"
	"github.com/theopenbee/openbee/desktop/locales"
	"rsc.io/qr"
)

const (
	telegramAPI   = pluginsAPI + "/telegram"
	settingsAPI   = telegramAPI + "/settings"
	setupAPI      = "/api/plugins/setup/"
	savedToken    = "****9f2c"
	setupTitle    = "Sign in with QR code"
	scanLine      = "Open Telegram and scan the code"
	firstLink     = "https://login.example.com/qr/1"
	secondLink    = "https://login.example.com/qr/2"
	savedFeedback = 2500 * time.Millisecond
	qrQuietZone   = 2
	qrDark        = 128
	qrSize        = 240
	qrSlack       = 0.5
	topScroll     = 5000
	sendOffset    = 40
	confirmOffset = 48
)

type settingsPut struct {
	Config  map[string]any `json:"config"`
	BotName *string        `json:"bot_name"`
}

func withSettings(st *fakeapi.State) {
	withPlugins(st)
	p := &st.Plugins[0]
	p.BotName = "openbee_bot"
	p.HasSetup = true
	p.SetupLabel = text(setupTitle, "扫码登录")
	p.Config = map[string]any{"workspace": "acme", "bot_token": savedToken, "max_threads": float64(4), "channels": []any{"general", "ops"}, "threads": true}
	p.Fields = []api.PluginConfigField{
		{Key: "workspace", Type: "string", Required: true, Label: text("Workspace", "工作区"), Help: text("The team the bot joins", "机器人加入的团队")},
		{Key: "bot_token", Type: "secret", Required: true, Label: text("Bot token", "机器人令牌")},
		{Key: "max_threads", Type: "int", Default: float64(8), Label: text("Max threads", "最大话题数")},
		{Key: "channels", Type: "list", Label: text("Channels", "频道")},
		{Key: "threads", Type: "bool", Label: text("Reply in threads", "在话题中回复"), Help: text("Keep replies under the original message", "回复保持在原消息下")},
	}
}

func openDetail(t *testing.T, seed func(st *fakeapi.State)) *fixture {
	t.Helper()
	f := open(t, wideW, settings.SectionPlugins, seed)
	f.installedTab()
	f.click("Telegram")
	f.wantShown(locales.PluginsDetailSettingsHint("Telegram"), locales.PluginsDetailInfo())
	return f
}

func (f *fixture) scrollToTop() {
	f.T.Scroll(paneX, paneY, 0, -topScroll)
	f.settle()
}

func (f *fixture) clickBeside(name string, dx float32) {
	f.t.Helper()
	r := f.rect(name)
	f.T.ClickAt(r.X+r.W+dx, r.Y+r.H/2)
	f.settle()
}

func (f *fixture) settingsPuts() []settingsPut {
	f.t.Helper()
	var out []settingsPut
	for _, call := range f.Fake.Calls(http.MethodPut, settingsAPI) {
		var body settingsPut
		if err := json.Unmarshal(call.Body, &body); err != nil {
			f.t.Fatalf("decode PUT body %q: %v", call.Body, err)
		}
		out = append(out, body)
	}
	return out
}

func (f *fixture) lastPut(want int) settingsPut {
	f.t.Helper()
	puts := f.settingsPuts()
	if len(puts) != want {
		f.t.Fatalf("settings PUTs = %d, want %d", len(puts), want)
	}
	return puts[want-1]
}

func wantConfig(t *testing.T, got settingsPut, want map[string]any) {
	t.Helper()
	if !maps.EqualFunc(got.Config, want, func(a, b any) bool { return a == b }) {
		t.Fatalf("PUT config = %v, want %v", got.Config, want)
	}
}

func (f *fixture) startSetup() string {
	f.t.Helper()
	f.click(locales.PluginsSetupStart())
	f.wantShown(locales.PluginsSetupScanHint(), locales.PluginsSetupAgain())
	var sid string
	f.Fix.Await(f.t, func() bool {
		sid = f.Fake.SetupSession("telegram")
		return sid != "" && f.Fake.SetupStreams(sid) == 1
	})
	f.settle()
	return sid
}

func TestFormSendsOnlyChanged(t *testing.T) {
	f := openDetail(t, withSettings)
	workspace := locales.PluginsDetailRequiredLabel("Workspace")
	f.wantShown(workspace, locales.PluginsDetailRequiredLabel("Bot token"), "Max threads", "Channels", "Reply in threads",
		locales.PluginsDetailBotName(), "The team the bot joins")
	f.typeInto(workspace, "acme-labs")
	f.clickIn(locales.PluginsDetailSave())

	put := f.lastPut(1)
	wantConfig(t, put, map[string]any{"workspace": "acme-labs"})
	if put.BotName != nil {
		t.Fatalf("PUT bot_name = %q, want it left out", *put.BotName)
	}
	f.wantShown(locales.PluginsToastSavedRestarting("Telegram"), locales.PluginsDetailSaved())
	f.scrollToTop()
	if got := f.fieldText(workspace); got != "acme-labs" {
		t.Fatalf("workspace after save = %q, want the saved value", got)
	}
	f.T.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	f.T.Advance(savedFeedback - time.Millisecond)
	f.settle()
	f.wantShown(locales.PluginsDetailSaved())
	f.T.Advance(time.Millisecond)
	f.settle()
	f.wantShown(locales.PluginsDetailSave())
	f.wantHidden(locales.PluginsDetailSaved())
}

func TestFormEmptySecretNotSent(t *testing.T) {
	f := openDetail(t, withSettings)
	token := locales.PluginsDetailRequiredLabel("Bot token")
	f.typeInto(token, "   ")
	f.typeInto(locales.PluginsDetailBotName(), "openbee_helper")
	f.clickIn(locales.PluginsDetailSave())
	put := f.lastPut(1)
	wantConfig(t, put, map[string]any{})
	if put.BotName == nil || *put.BotName != "openbee_helper" {
		t.Fatalf("PUT bot_name = %v, want openbee_helper", put.BotName)
	}

	f.scrollToTop()
	f.typeInto(token, " 123:new-token ")
	f.clickIn(locales.PluginsDetailSaved())
	put = f.lastPut(2)
	wantConfig(t, put, map[string]any{"bot_token": "123:new-token"})
	if put.BotName != nil {
		t.Fatalf("second PUT bot_name = %q, want it left out", *put.BotName)
	}
}

func TestFormIntValidation(t *testing.T) {
	f := openDetail(t, withSettings)
	f.typeInto("Max threads", "1.5")
	f.clickIn(locales.PluginsDetailSave())
	f.wantShown(locales.PluginsDetailIntInvalid("Max threads"))
	if n := len(f.settingsPuts()); n != 0 {
		t.Fatalf("settings PUTs = %d with an invalid whole number, want none", n)
	}
	if !f.T.Focused("Max threads") {
		t.Fatal("the invalid field does not have the focus")
	}

	f.scrollToTop()
	f.typeInto("Max threads", "12")
	f.wantHidden(locales.PluginsDetailIntInvalid("Max threads"))
	f.clickIn(locales.PluginsDetailSave())
	wantConfig(t, f.lastPut(1), map[string]any{"max_threads": float64(12)})
}

func TestFormNothingToSaveToast(t *testing.T) {
	f := openDetail(t, withSettings)
	f.clickIn(locales.PluginsDetailSave())
	f.wantShown(locales.PluginsDetailNoChanges())
	if n := len(f.settingsPuts()); n != 0 {
		t.Fatalf("settings PUTs = %d without changes, want none", n)
	}
}

func TestFormReset(t *testing.T) {
	f := openDetail(t, withSettings)
	workspace := locales.PluginsDetailRequiredLabel("Workspace")
	f.typeInto(workspace, "changed")
	f.typeInto(locales.PluginsDetailBotName(), "someone_else")
	f.clickIn(locales.PluginsDetailReset())
	f.scrollToTop()
	if got := f.fieldText(workspace); got != "acme" {
		t.Fatalf("workspace after reset = %q, want the saved value", got)
	}
	if got := f.fieldText(locales.PluginsDetailBotName()); got != "openbee_bot" {
		t.Fatalf("bot name after reset = %q, want the saved value", got)
	}
	f.clickIn(locales.PluginsDetailSave())
	f.wantShown(locales.PluginsDetailNoChanges())
	if n := len(f.settingsPuts()); n != 0 {
		t.Fatalf("settings PUTs = %d after reset, want none", n)
	}
}

func TestDetailActions(t *testing.T) {
	f := openDetail(t, withSettings)
	release := f.Fake.HoldNext(http.MethodPost, telegramAPI+"/restart")
	t.Cleanup(release)
	f.press(locales.PluginsActionRestart())
	f.pump(func() bool { return f.posted(telegramAPI+"/restart") == 1 })
	f.press(locales.PluginsActionDisable())
	if n := f.posted(telegramAPI + "/disable"); n != 0 {
		t.Fatalf("Disable posted %d times while a restart was running", n)
	}
	release()
	f.settle()
	f.wantShown(locales.PluginsToastRestarted("Telegram"))

	f.click(locales.PluginsActionUninstall())
	f.wantShown(locales.PluginsUninstallDialogTitle("Telegram"))
	f.clickBeside(locales.CommonCancel(), confirmOffset)
	f.wantShown(locales.PluginsInstalledTitle(), locales.PluginsStateUninstalling())
	f.wantHidden(locales.PluginsUninstallDialogTitle("Telegram"), locales.PluginsDetailInfo())
}

func TestDetailNotFound(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withPlugins)
	f.installedTab()
	f.Fake.FailNext(http.MethodGet, pluginsAPI+"/discord", http.StatusNotFound,
		map[string]any{"error": "plugin is not installed", "code": "plugin_not_found", "params": map[string]any{"id": "discord"}})
	f.click("Discord")
	f.wantShown(locales.PluginsDetailNotFound(), locales.PluginsDetailNotFoundHint(), locales.PluginsDetailBack())
	f.wantHidden(locales.PluginsDetailInfo(), locales.PluginsDetailLoadError())

	f.click(locales.PluginsEmptyInstalledAction())
	f.wantShown(locales.PluginsAllTags(), locales.PluginsRefreshCatalog())
	f.wantHidden(locales.PluginsDetailNotFound())
}

func TestDetailLoadErrorRetry(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withPlugins)
	f.installedTab()
	f.Fake.FailNext(http.MethodGet, pluginsAPI+"/discord", http.StatusInternalServerError, map[string]string{"error": "database is locked"})
	f.click("Discord")
	f.wantShown(locales.PluginsDetailLoadError(), "database is locked")
	f.click(locales.CommonRetry())
	f.wantShown(locales.PluginsDetailFailedTitle(), discordFailure, locales.PluginsDetailRestartNow())
	f.wantHidden(locales.PluginsDetailLoadError())
}

func TestSetupShowsQRForLatestLink(t *testing.T) {
	f := openDetail(t, withSettings)
	f.wantShown(setupTitle, locales.PluginsSetupCardHint())
	f.T.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	sid := f.startSetup()
	f.Fake.SetupEmit(sid, api.SetupEvent{Type: "line", Text: scanLine})
	f.Fake.SetupEmit(sid, api.SetupEvent{Type: "line", Text: firstLink})
	f.Fake.SetupEmit(sid, api.SetupEvent{Type: "line", Text: secondLink})
	f.awaitShown(secondLink)
	f.wantShown(scanLine, locales.PluginsSetupRefreshHint())
	f.wantHidden(firstLink)

	box := f.rect(secondLink)
	if math.Abs(float64(box.W-qrSize)) > qrSlack || math.Abs(float64(box.H-qrSize)) > qrSlack {
		t.Fatalf("QR code box = %+v, want a %v DIP square", box, qrSize)
	}
	want := qrCode(t, secondLink)
	other := qrCode(t, firstLink)
	frame := f.T.Image()
	step := box.W / float32(want.Size+2*qrQuietZone)
	differs := false
	for y := -qrQuietZone; y < want.Size+qrQuietZone; y++ {
		for x := -qrQuietZone; x < want.Size+qrQuietZone; x++ {
			px := int(box.X + (float32(x+qrQuietZone)+0.5)*step)
			py := int(box.Y + (float32(y+qrQuietZone)+0.5)*step)
			r, _, _, _ := frame.At(px, py).RGBA()
			if dark := r>>8 < qrDark; dark != want.Black(x, y) {
				t.Fatalf("QR module (%d, %d) dark = %v, want the code for %q", x, y, dark, secondLink)
			}
			differs = differs || want.Black(x, y) != other.Black(x, y)
		}
	}
	if !differs {
		t.Fatalf("the code for %q also encodes %q", secondLink, firstLink)
	}
}

func qrCode(t *testing.T, text string) *qr.Code {
	t.Helper()
	code, err := qr.Encode(text, qr.L)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func TestSetupSendInput(t *testing.T) {
	f := openDetail(t, withSettings)
	sid := f.startSetup()
	f.Fake.SetupEmit(sid, api.SetupEvent{Type: "line", Text: "Enter the code sent to your phone"})
	f.awaitShown("Enter the code sent to your phone")
	inputPath := setupAPI + sid + "/input"

	reply := locales.PluginsSetupInputLabel()
	f.clickBeside(reply, sendOffset)
	f.click(reply)
	f.T.Key(0, ui.KeyEnter)
	f.settle()
	if n := f.posted(inputPath); n != 0 {
		t.Fatalf("an empty reply posted %d times", n)
	}
	f.typeInto(reply, " 123456 ")
	f.clickBeside(reply, sendOffset)
	calls := f.Fake.Calls(http.MethodPost, inputPath)
	if len(calls) != 1 {
		t.Fatalf("input posts = %d, want 1", len(calls))
	}
	var body map[string]string
	if err := json.Unmarshal(calls[0].Body, &body); err != nil || body["text"] != "123456" {
		t.Fatalf("input body = %s (%v), want the trimmed reply", calls[0].Body, err)
	}
	if got := f.fieldText(reply); got != "" {
		t.Fatalf("reply after sending = %q, want it cleared", got)
	}

	f.Fake.SetupEmit(sid, api.SetupEvent{Type: "done", OK: true})
	f.awaitShown(locales.PluginsSetupSuccess())
	f.wantHidden(locales.PluginsSetupScanHint())
	if n := len(f.Fake.Calls(http.MethodDelete, setupAPI+sid)); n != 0 {
		t.Fatalf("a finished session was cancelled %d times", n)
	}
}

func TestSetupCloseCancels(t *testing.T) {
	f := openDetail(t, withSettings)
	sid := f.startSetup()
	f.wantShown(locales.CommonCancel())
	f.click(locales.CommonCancel())
	f.wantHidden(locales.PluginsSetupScanHint())
	f.Fix.Await(t, func() bool { return len(f.Fake.Calls(http.MethodDelete, setupAPI+sid)) == 1 })
	if n := f.Fake.SetupStreams(sid); n != 0 {
		t.Fatalf("%d setup streams still open after closing", n)
	}
}
