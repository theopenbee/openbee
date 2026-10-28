package settings_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/settings"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	pluginsAPI     = "/api/plugins"
	operationsAPI  = "/api/plugins/operations"
	installAPI     = "/api/plugins/install"
	telegramPkg    = "@openbee/plugin-telegram"
	linearPkg      = "@openbee/plugin-linear"
	discordFailure = "discord: 401 Unauthorized"
	backInset      = 20
	narrowW        = 700
	detailGap      = 16
)

func text(en, zh string) api.LocalizedText {
	return api.LocalizedText{"en": en, "zh": zh}
}

func listed(id, pkg, version, state string, enabled bool, en, zh string) api.PluginDetail {
	return api.PluginDetail{PluginStatus: api.PluginStatus{ID: id, Package: pkg, Source: "npm", Version: version,
		Name: text(en, zh), Enabled: enabled, State: state}}
}

func withPlugins(st *fakeapi.State) {
	configured(st)
	st.Catalog = api.PluginCatalog{
		SchemaVersion: 1,
		Tags: []api.CatalogTag{
			{ID: "chat", Name: text("Messaging", "消息")},
			{ID: "china", Name: text("China", "中国")},
			{ID: "email", Name: text("Email", "邮件")},
		},
		Plugins: []api.CatalogPlugin{
			{ID: "telegram", Package: telegramPkg, Version: "1.3.0", Official: true, Name: text("Telegram", "Telegram"),
				Description: text("Chat with your workers from Telegram.", "在 Telegram 里和员工对话。"), Tags: []string{"chat"},
				Installed: true, InstalledVersion: "1.2.0", UpdateAvailable: true, Installable: true},
			{ID: "discord", Package: "@openbee/plugin-discord", Version: "1.1.0", Name: text("Discord", "Discord"), Tags: []string{"chat"},
				Installed: true, InstalledVersion: "1.1.0", Installable: true},
			{ID: "dingtalk", Package: "@openbee/plugin-dingtalk", Version: "0.8.0", Name: text("DingTalk", "钉钉"), Tags: []string{"china"},
				Installed: true, InstalledVersion: "0.8.0", Installable: true},
			{ID: "feishu", Package: "@openbee/plugin-feishu", Version: "0.9.2", Name: text("Feishu", "飞书"), Tags: []string{"chat", "china"},
				Installed: true, InstalledVersion: "0.9.2", Installable: true},
			{ID: "wechat", Package: "@openbee/plugin-wechat", Version: "0.3.0", Name: text("WeChat", "微信"), Tags: []string{"china"},
				MinOpenbeeVersion: "9.0.0"},
			{ID: "linear", Package: linearPkg, Version: "0.4.0", Official: true, Name: text("Linear", "Linear"),
				Description: text("Turn Linear issues into worker tasks.", "把 Linear 工单交给员工。"), Installable: true},
		},
	}
	discord := listed("discord", "@openbee/plugin-discord", "1.1.0", "failed", true, "Discord", "Discord")
	discord.Error = discordFailure
	st.Plugins = []api.PluginDetail{
		listed("telegram", telegramPkg, "1.2.0", "running", true, "Telegram", "Telegram"),
		discord,
		listed("feishu", "@openbee/plugin-feishu", "0.9.2", "not_configured", false, "Feishu", "飞书"),
		listed("dingtalk", "@openbee/plugin-dingtalk", "0.8.0", "disabled", false, "DingTalk", "钉钉"),
	}
}

func (f *fixture) installedTab() {
	f.t.Helper()
	f.click(locales.PluginsTabsInstalled())
	f.wantShown(locales.PluginsInstalledTitle())
}

func (f *fixture) armed() {
	f.t.Helper()
	f.Fix.Await(f.t, func() bool { return f.Fix.Clock.Sleepers() == 1 })
}

func (f *fixture) poll(path string) {
	f.t.Helper()
	f.armed()
	n := len(f.Fake.Calls(http.MethodGet, path))
	f.Fix.Clock.Advance(time.Second)
	f.Fix.Await(f.t, func() bool { return len(f.Fake.Calls(http.MethodGet, path)) > n })
	f.settle()
}

func (f *fixture) awaitShown(text string) {
	f.t.Helper()
	f.Fix.Await(f.t, func() bool {
		f.T.Frame()
		return f.shows(text)
	})
	f.settle()
}

func (f *fixture) lastOperation() api.PluginOperation {
	f.t.Helper()
	ops := f.Fake.Snapshot().Operations
	if len(ops) == 0 {
		f.t.Fatal("no plugin operation was started")
	}
	return ops[len(ops)-1]
}

func (f *fixture) posted(path string) int {
	return len(f.Fake.Calls(http.MethodPost, path))
}

func (f *fixture) press(name string) {
	f.t.Helper()
	if err := f.T.Click(name); err != nil {
		f.t.Fatalf("click %q: %v (texts %q)", name, err, f.T.Texts())
	}
	f.pump(func() bool { return true })
}

func (f *fixture) pump(cond func() bool) {
	f.t.Helper()
	f.Fix.Pump(f.t, func() bool {
		f.T.Frame()
		return cond()
	})
}

func (f *fixture) advance(d time.Duration, path string) {
	f.t.Helper()
	f.armed()
	n := len(f.Fake.Calls(http.MethodGet, path))
	f.Fix.Clock.Advance(d)
	f.Fix.Await(f.t, func() bool { return len(f.Fake.Calls(http.MethodGet, path)) > n })
	f.settle()
}

func (f *fixture) reads(path string) int {
	n := 0
	for _, call := range f.Fake.Calls(http.MethodGet, path) {
		if call.Path == path {
			n++
		}
	}
	return n
}

func TestMarketplaceFilterByTag(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withPlugins)
	f.wantShown(locales.PluginsTabsMarket(), locales.PluginsAllTags(), "Messaging", "China", "Telegram", "Discord", "Feishu", "DingTalk", "WeChat", "Linear")
	f.wantHidden("Email")

	f.click("China")
	f.wantShown("Feishu", "DingTalk", "WeChat")
	f.wantHidden("Telegram", "Discord", "Linear")

	f.click("Messaging")
	f.wantShown("Telegram", "Discord", "Feishu")
	f.wantHidden("DingTalk", "WeChat", "Linear")

	f.click(locales.PluginsAllTags())
	f.wantShown("Telegram", "WeChat", "Linear")
}

func TestMarketplaceSearch(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withPlugins)
	f.typeInto(locales.PluginsSearch(), "issues")
	f.wantShown("Linear")
	f.wantHidden("Telegram", "Feishu")

	f.typeInto(locales.PluginsSearch(), "钉钉")
	f.wantShown("DingTalk")
	f.wantHidden("Linear", "Feishu")

	f.typeInto(locales.PluginsSearch(), "nothing like this")
	f.wantShown(locales.PluginsNoResults(), locales.PluginsNoResultsHint())
	f.click("China")
	f.click(locales.PluginsClearFilters())
	f.wantHidden(locales.PluginsNoResults())
	f.wantShown("Telegram", "WeChat", "Linear")
	if got := f.fieldText(locales.PluginsSearch()); got != "" {
		t.Fatalf("search = %q after Clear filters", got)
	}
}

func TestInstallShowsProgress(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, func(st *fakeapi.State) {
		withPlugins(st)
		st.Operations = []api.PluginOperation{{ID: "op-1", Kind: "install", Spec: linearPkg + "@0.4.0", Status: "running", CreatedAt: time.Now()}}
	})
	f.wantShown(locales.PluginsActionInstallingShort(), "0:00")
	f.wantHidden(locales.PluginsActionInstall())
	f.T.Advance(time.Second)
	f.wantShown("0:01")

	f.Fake.FinishOperation("op-1", true, "")
	f.poll(operationsAPI)
	f.awaitShown(locales.PluginsInstallDone("Linear"))
	f.wantHidden(locales.PluginsActionInstallingShort())
	f.wantShown(locales.PluginsInstalledVersion("0.4.0"))
}

func TestUpdateButtonWhenUpdateAvailable(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withPlugins)
	f.wantShown(locales.PluginsUpdateAvailable("1.2.0"), locales.PluginsInstalledVersion("1.1.0"))
	f.clickIn(locales.PluginsActionUpdate("1.3.0"))
	calls := f.Fake.Calls(http.MethodPost, installAPI)
	if len(calls) != 1 {
		t.Fatalf("install posts = %d, want 1", len(calls))
	}
	var body map[string]string
	if err := json.Unmarshal(calls[0].Body, &body); err != nil || body["spec"] != telegramPkg+"@1.3.0" {
		t.Fatalf("install body = %s (%v), want the catalog version", calls[0].Body, err)
	}
	f.wantShown(locales.PluginsActionInstallingShort())
	f.wantHidden(locales.PluginsActionUpdate("1.3.0"))
}

func TestIncompatibleText(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withPlugins)
	f.wantShown(locales.PluginsActionIncompatible(), locales.PluginsActionInstall())
	f.click("China")
	f.wantShown(locales.PluginsActionIncompatible())
	f.wantHidden(locales.PluginsActionInstall())
}

func TestInstalledToggleDisabledWhenNotConfigured(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withPlugins)
	f.installedTab()
	f.wantShown(locales.PluginsInstalledSummary(4, "1"), locales.PluginsStateRunning(), locales.PluginsStateFailed(),
		locales.PluginsStateNotConfigured(), locales.PluginsStateDisabled(), discordFailure, "v1.2.0", locales.PluginsColumnsBotName())

	f.click(locales.PluginsActionToggle("Feishu"))
	if n := f.posted(pluginsAPI + "/feishu/enable"); n != 0 {
		t.Fatalf("enable posted %d times for a plugin that is not configured", n)
	}
	f.click(locales.PluginsActionToggle("DingTalk"))
	if n := f.posted(pluginsAPI + "/dingtalk/enable"); n != 1 {
		t.Fatalf("enable posts for DingTalk = %d, want 1", n)
	}
	f.wantShown(locales.PluginsToastEnabled("DingTalk"))
}

func TestUninstallDialog(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withPlugins)
	f.installedTab()
	f.click(locales.PluginsActionMoreFor("Discord"))
	f.wantShown(locales.PluginsActionRestart())
	f.click(locales.PluginsActionUninstall())
	f.wantShown(locales.PluginsUninstallDialogTitle("Discord"), locales.PluginsUninstallDialogDescription())

	f.Fake.FailNext(http.MethodDelete, pluginsAPI+"/discord", http.StatusConflict, map[string]string{"error": "plugin is busy"})
	f.click(locales.PluginsUninstallDialogConfirm())
	f.wantShown(locales.PluginsUninstallDialogError(), "plugin is busy")

	f.click(locales.PluginsUninstallDialogConfirm())
	f.wantHidden(locales.PluginsUninstallDialogTitle("Discord"))
	f.wantShown(locales.PluginsStateUninstalling())

	f.Fake.FinishOperation(f.lastOperation().ID, true, "")
	f.poll(operationsAPI)
	f.awaitShown(locales.PluginsUninstallDone("Discord"))
	f.wantHidden(locales.PluginsStateUninstalling(), discordFailure)
	f.wantShown(locales.PluginsInstalledSummary(3, "1"))
}

func TestInstallFromPackageSuccessConfigure(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withPlugins)
	f.click(locales.PluginsInstallDialogOpen())
	f.wantShown(locales.PluginsInstallDialogTitle(), locales.PluginsInstallDialogSpecHelp(), locales.CommonCancel())
	f.typeInto(locales.PluginsInstallDialogSpec(), "  "+linearPkg+"@0.4.0 ")
	f.click(locales.PluginsInstallDialogSubmit())
	op := f.lastOperation()
	if op.Spec != linearPkg+"@0.4.0" {
		t.Fatalf("install spec = %q, want it trimmed", op.Spec)
	}
	f.wantShown(locales.PluginsInstallDialogInstalling(), locales.PluginsInstallDialogClose())
	f.wantHidden(locales.CommonCancel())

	f.Fake.FinishOperation(op.ID, true, "")
	f.poll(operationsAPI)
	f.awaitShown(locales.PluginsInstallDialogConfigure())
	f.wantShown(locales.PluginsInstallDialogDone(), locales.PluginsInstallDialogDoneHint(), locales.PluginsInstallDone("Linear"))
	f.wantHidden(locales.PluginsInstallDialogInstalling())

	f.click(locales.PluginsInstallDialogConfigure())
	f.wantHidden(locales.PluginsInstallDialogTitle(), locales.PluginsTabsMarket())
	f.wantShown("Linear", locales.PluginsSubtitle())
	title := f.rect("Linear")
	f.T.ClickAt(title.X+backInset, title.Y-detailGap-theme.ControlSM/2)
	f.settle()
	f.wantShown(locales.PluginsInstalledTitle(), locales.PluginsInstalledSummary(5, "1"))
}

func TestInstallFromPackageFailure(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withPlugins)
	f.click(locales.PluginsInstallDialogOpen())
	f.typeInto(locales.PluginsInstallDialogSpec(), "@acme/missing@1.0.0")
	f.T.Key(0, ui.KeyEnter)
	f.settle()
	f.Fake.FinishOperation(f.lastOperation().ID, false, "npm ERR! 404 Not Found")
	f.poll(operationsAPI)
	f.awaitShown(locales.PluginsInstallDialogFailed())
	f.wantShown("npm ERR! 404 Not Found", locales.CommonCancel(), locales.PluginsInstallDialogSubmit())
}

func TestInstallDialogSurvivesResize(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withPlugins)
	f.click(locales.PluginsInstallDialogOpen())
	f.typeInto(locales.PluginsInstallDialogSpec(), linearPkg)
	f.T.SetSize(narrowW, wideH)
	f.settle()
	f.wantShown(locales.PluginsInstallDialogTitle())
	if got := f.fieldText(locales.PluginsInstallDialogSpec()); got != linearPkg {
		t.Fatalf("spec = %q after crossing the two-pane breakpoint, want the draft kept", got)
	}
}

func TestUninstallRequestKeepsOtherRowsActive(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withPlugins)
	f.installedTab()
	deletePath := pluginsAPI + "/discord"
	release := f.Fake.HoldNext(http.MethodDelete, deletePath)
	t.Cleanup(release)
	f.click(locales.PluginsActionMoreFor("Discord"))
	f.click(locales.PluginsActionUninstall())
	f.press(locales.PluginsUninstallDialogConfirm())
	f.pump(func() bool { return len(f.Fake.Calls(http.MethodDelete, deletePath)) == 1 })
	f.press(locales.CommonCancel())
	f.wantHidden(locales.PluginsUninstallDialogTitle("Discord"))

	f.press(locales.PluginsActionToggle("Discord"))
	f.press(locales.PluginsActionMoreFor("Telegram"))
	f.press(locales.PluginsActionRestart())
	f.pump(func() bool { return f.posted(pluginsAPI+"/telegram/restart") == 1 && !f.Store.Plugins().Acting() })
	f.press(locales.PluginsActionToggle("Telegram"))
	f.pump(func() bool { return f.posted(pluginsAPI+"/telegram/disable") == 1 })
	if n := f.posted(pluginsAPI + "/discord/disable"); n != 0 {
		t.Fatalf("the plugin being uninstalled was toggled %d times while its DELETE ran", n)
	}
	release()
	f.settle()
	f.wantShown(locales.PluginsStateUninstalling(), locales.PluginsToastRestarted("Telegram"), locales.PluginsToastDisabled("Telegram"))
}

func TestPluginDetailWatchedOnlyWhileOpen(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, func(st *fakeapi.State) {
		withPlugins(st)
		st.Plugins[0].State = "starting"
	})
	detailPath := pluginsAPI + "/telegram"
	f.installedTab()
	f.click("Telegram")
	f.wantShown(locales.PluginsDetailBack())
	f.Store.Plugins().Detail("telegram").Ensure()
	f.settle()
	f.advance(2*time.Second, detailPath)

	title := f.rect("Telegram")
	f.T.ClickAt(title.X+backInset, title.Y-detailGap-theme.ControlSM/2)
	f.settle()
	f.wantShown(locales.PluginsInstalledTitle())
	n := f.reads(detailPath)
	f.advance(2*time.Second, pluginsAPI)
	if got := f.reads(detailPath); got != n {
		t.Fatalf("detail reads went from %d to %d after leaving the detail, want no poll", n, got)
	}

	f.click("Telegram")
	f.wantShown(locales.PluginsDetailBack())
	f.T.Key(0, ui.KeyEscape)
	f.settle()
	f.Root.OpenSettings(settings.SectionPlugins)
	f.settle()
	f.advance(2*time.Second, pluginsAPI)
	if got := f.reads(detailPath); got != n {
		t.Fatalf("detail reads went from %d to %d after closing on the detail, want no poll", n, got)
	}
}
