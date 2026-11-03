package settings_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/views/settings"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	claudeEngineAPI = pluginsAPI + "/claude"
	codexMissing    = "codex not found on PATH"
	enginePkg       = "@openbee/engine-"
)

func engineEntry(id, name string, installed bool) api.CatalogPlugin {
	entry := api.CatalogPlugin{Category: "engine", ID: id, Package: enginePkg + id, Version: "0.1.0", Name: text(name, name),
		Description: text("Runs workers with "+name+".", "用 "+name+" 运行员工。"), Installable: true}
	if installed {
		entry.Installed, entry.InstalledVersion = true, "0.1.0"
	}
	return entry
}

func installedEngine(id, name, state string) api.PluginDetail {
	d := listed(id, enginePkg+id, "0.1.0", state, true, name, name)
	d.Type = "engine"
	return d
}

func withEngines(st *fakeapi.State) {
	withPlugins(st)
	st.Catalog.Plugins = append(st.Catalog.Plugins,
		engineEntry("claude", "Claude Code", true),
		engineEntry("codex", "Codex", true),
		engineEntry("pi", "Pi", false),
	)
	codex := installedEngine("codex", "Codex", "failed")
	codex.Error = codexMissing
	st.Plugins = append(st.Plugins, installedEngine("claude", "Claude Code", "ready"), codex)
}

func withEngineSettings(st *fakeapi.State) {
	withEngines(st)
	claude := &st.Plugins[4]
	claude.HasSetup = true
	claude.Fields = []api.PluginConfigField{{Key: "binary", Type: "string", Label: text("Binary", "可执行文件")}}
	claude.Config = map[string]any{"binary": "claude"}
}

func (f *fixture) openEngine(name string) {
	f.t.Helper()
	f.installedTab()
	f.click(name)
	f.wantShown(locales.PluginsDetailInfo())
}

func (f *fixture) wantBefore(upper, lower string) {
	f.t.Helper()
	if a, b := f.rect(upper).Y, f.rect(lower).Y; a >= b {
		f.t.Fatalf("%q at y=%v is not above %q at y=%v", upper, a, lower, b)
	}
}

func TestReadyStateLooksHealthy(t *testing.T) {
	if got := settings.StateTone("ready"); got != kit.ToneSuccess {
		t.Fatalf("tone of ready = %v, want the same tone as running (%v)", got, kit.ToneSuccess)
	}
	if got, want := settings.StateTone("ready"), settings.StateTone("running"); got != want {
		t.Fatalf("ready tone %v differs from running tone %v", got, want)
	}
}

func TestInstalledGroupsPluginsByType(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withEngines)
	f.installedTab()
	f.wantShown(locales.PluginsTypesChannel(), locales.PluginsTypesEngine(), "Claude Code", "Codex", locales.PluginsStateReady(), codexMissing)
	f.wantBefore(locales.PluginsTypesChannel(), "Telegram")
	f.wantBefore("DingTalk", locales.PluginsTypesEngine())
	f.wantBefore(locales.PluginsTypesEngine(), "Claude Code")
	f.wantBefore("Claude Code", "Codex")
}

func TestInstalledSummaryCountsReadyEngines(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withEngines)
	f.installedTab()
	f.wantShown(locales.PluginsInstalledSummary(6, "2"))
}

func TestInstalledWithoutEnginesOnlyShowsChannels(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withPlugins)
	f.installedTab()
	f.wantShown(locales.PluginsTypesChannel(), "Telegram")
	f.wantHidden(locales.PluginsTypesEngine())
}

func TestMarketplaceGroupsByType(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withEngines)
	f.wantShown(locales.PluginsTypesChannel(), locales.PluginsTypesEngine(), "Telegram", "Linear", "Claude Code", "Pi")
	f.wantBefore("Linear", locales.PluginsTypesEngine())
	f.wantBefore(locales.PluginsTypesEngine(), "Claude Code")
	f.wantBefore(locales.PluginsTypesChannel(), "Telegram")

	f.typeInto(locales.PluginsSearch(), "Runs workers")
	f.wantShown(locales.PluginsTypesEngine(), "Claude Code", "Codex", "Pi")
	f.wantHidden(locales.PluginsTypesChannel(), "Telegram")
}

func TestMarketplaceUsesCatalogCategoryNames(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, func(st *fakeapi.State) {
		withEngines(st)
		st.Catalog.Categories = []api.CatalogTag{{ID: "engine", Name: text("AI engines", "AI 引擎")}}
	})
	f.wantShown("AI engines", locales.PluginsTypesChannel())
	f.wantHidden(locales.PluginsTypesEngine())
}

func TestEngineDetailHasNoBotNameOrSetup(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withEngineSettings)
	f.openEngine("Claude Code")
	f.wantShown(locales.PluginsDetailSettingsHintEngine(), "Binary", locales.PluginsStateReady(), locales.PluginsDetailSave())
	f.wantHidden(locales.PluginsDetailBotName(), locales.PluginsDetailBotNameHelp(), locales.PluginsSetupStart(), locales.PluginsSetupCardHint(),
		locales.PluginsDetailSettingsHint("Claude Code"))
}

func TestEngineDetailSaveNeverSendsBotName(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withEngineSettings)
	f.openEngine("Claude Code")
	f.typeInto("Binary", "/opt/claude")
	f.clickIn(locales.PluginsDetailSave())
	calls := f.Fake.Calls(http.MethodPut, claudeEngineAPI+"/settings")
	if len(calls) != 1 {
		t.Fatalf("settings PUTs = %d, want 1", len(calls))
	}
	var body settingsPut
	if err := json.Unmarshal(calls[0].Body, &body); err != nil {
		t.Fatalf("decode PUT body %q: %v", calls[0].Body, err)
	}
	wantConfig(t, body, map[string]any{"binary": "/opt/claude"})
	if body.BotName != nil {
		t.Fatalf("PUT bot_name = %q, want it left out for an engine", *body.BotName)
	}
	f.wantShown(locales.PluginsToastSaved())
	f.wantHidden(locales.PluginsToastSavedRestarting("Claude Code"), locales.PluginsToastSavedEnableHint())
}

func TestEngineDetailWithoutFieldsHasNoSettingsForm(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withEngines)
	f.openEngine("Claude Code")
	f.wantShown(locales.PluginsDetailInfo(), locales.PluginsActionDisable(), locales.PluginsActionRestart(), locales.PluginsActionUninstall())
	f.wantHidden(locales.PluginsDetailSettingsHintEngine(), locales.PluginsDetailSave(), locales.PluginsDetailBotName(), locales.PluginsDetailReset())
}

func TestFailedEngineDetailShowsTheCliError(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withEngines)
	f.openEngine("Codex")
	f.wantShown(locales.PluginsDetailFailedTitle(), codexMissing, locales.PluginsDetailRestartNow())
	f.click(locales.PluginsDetailRestartNow())
	f.Fix.Await(f.t, func() bool { return f.posted(pluginsAPI+"/codex/restart") == 1 })
	f.settle()
	f.wantShown(locales.PluginsStateReady())
	f.wantHidden(locales.PluginsDetailFailedTitle(), codexMissing)
}

func TestChannelDetailKeepsBotNameAndSetup(t *testing.T) {
	f := openDetail(t, func(st *fakeapi.State) {
		withSettings(st)
		addClaude(st)
	})
	f.wantShown(locales.PluginsDetailBotName(), setupTitle, locales.PluginsDetailSettingsHint("Telegram"))
	f.wantHidden(locales.PluginsDetailSettingsHintEngine())
}

func addClaude(st *fakeapi.State) {
	st.Catalog.Plugins = append(st.Catalog.Plugins, engineEntry("claude", "Claude Code", true))
	st.Plugins = append(st.Plugins, installedEngine("claude", "Claude Code", "ready"))
}

func TestEngineUninstallDialogWarnsAboutWorkers(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withEngines)
	f.installedTab()
	f.click(locales.PluginsActionMoreFor("Claude Code"))
	f.click(locales.PluginsActionUninstall())
	f.wantShown(locales.PluginsUninstallDialogTitle("Claude Code"), locales.PluginsUninstallDialogDescriptionEngine())
	f.wantHidden(locales.PluginsUninstallDialogDescription())
}

func TestChannelUninstallDialogKeepsItsOwnText(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withEngines)
	f.installedTab()
	f.click(locales.PluginsActionMoreFor("Telegram"))
	f.click(locales.PluginsActionUninstall())
	f.wantShown(locales.PluginsUninstallDialogDescription())
	f.wantHidden(locales.PluginsUninstallDialogDescriptionEngine())
}

func TestEngineDetailUninstallDialogWarnsAboutWorkers(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withEngines)
	f.openEngine("Claude Code")
	f.click(locales.PluginsActionUninstall())
	f.wantShown(locales.PluginsUninstallDialogDescriptionEngine())
	f.wantHidden(locales.PluginsUninstallDialogDescription())
}

func TestInstallEngineFromPackageOffersViewNotConfigure(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withEngines)
	f.click(locales.PluginsInstallDialogOpen())
	f.typeInto(locales.PluginsInstallDialogSpec(), enginePkg+"pi@0.1.0")
	f.click(locales.PluginsInstallDialogSubmit())
	op := f.lastOperation()
	f.Fake.FinishOperation(op.ID, true, "")
	f.poll(operationsAPI)
	f.awaitShown(locales.PluginsInstallDialogView())
	f.wantShown(locales.PluginsInstallDialogDone(), locales.PluginsInstallDialogDoneHintEngine())
	f.wantHidden(locales.PluginsInstallDialogDoneHint(), locales.PluginsInstallDialogConfigure())

	f.click(locales.PluginsInstallDialogView())
	f.wantHidden(locales.PluginsInstallDialogTitle())
	f.wantShown(locales.PluginsDetailInfo(), locales.PluginsStateReady())
}

func TestInstallChannelFromPackageKeepsConfigure(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withEngines)
	f.click(locales.PluginsInstallDialogOpen())
	f.typeInto(locales.PluginsInstallDialogSpec(), linearPkg+"@0.4.0")
	f.click(locales.PluginsInstallDialogSubmit())
	f.Fake.FinishOperation(f.lastOperation().ID, true, "")
	f.poll(operationsAPI)
	f.awaitShown(locales.PluginsInstallDialogConfigure())
	f.wantShown(locales.PluginsInstallDialogDoneHint())
	f.wantHidden(locales.PluginsInstallDialogDoneHintEngine(), locales.PluginsInstallDialogView())
}

func TestEngineToggleKeepsReadyState(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withEngines)
	f.installedTab()
	f.click(locales.PluginsActionToggle("Claude Code"))
	f.Fix.Await(f.t, func() bool { return f.posted(claudeEngineAPI+"/disable") == 1 })
	f.settle()
	f.click(locales.PluginsActionToggle("Claude Code"))
	f.Fix.Await(f.t, func() bool { return f.posted(claudeEngineAPI+"/enable") == 1 })
	f.settle()
	f.awaitShown(locales.PluginsToastEnabled("Claude Code"))
	f.wantShown(locales.PluginsInstalledSummary(6, "2"))
}
