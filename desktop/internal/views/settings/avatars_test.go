package settings_test

import (
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/views/settings"
	"github.com/theopenbee/openbee/desktop/locales"
)

const avatarPkg = "@openbee/avatars-"

func avatarEntry(id, name string, installed bool) api.CatalogPlugin {
	entry := api.CatalogPlugin{Category: "avatar", ID: id, Package: avatarPkg + id, Version: "1.0.0", Name: text(name, name),
		Description: text("Worker avatars drawn by "+name+".", name+" 画的员工头像。"), Installable: true}
	if installed {
		entry.Installed, entry.InstalledVersion = true, "1.0.0"
	}
	return entry
}

func withAvatarPacks(st *fakeapi.State) {
	withEngines(st)
	st.Catalog.Plugins = append(st.Catalog.Plugins, avatarEntry("mfers", "mfers", true), avatarEntry("robots", "Robots", false))
	st.AddAvatarPack("mfers", text("mfers", "mfers"), 4)
	st.Plugins[len(st.Plugins)-1].Package = avatarPkg + "mfers"
}

func TestInstalledGroupsAvatarPlugins(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withAvatarPacks)
	f.installedTab()
	f.wantShown(locales.PluginsTypesChannel(), locales.PluginsTypesEngine(), locales.PluginsTypesAvatar(), "mfers")
	f.wantBefore(locales.PluginsTypesEngine(), locales.PluginsTypesAvatar())
	f.wantBefore(locales.PluginsTypesAvatar(), "mfers")
}

func TestMarketplaceGroupsAvatarPlugins(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withAvatarPacks)
	f.wantShown(locales.PluginsTypesAvatar(), "Robots")
	f.wantBefore(locales.PluginsTypesEngine(), locales.PluginsTypesAvatar())
	f.wantBefore(locales.PluginsTypesAvatar(), "Robots")
}

func TestAvatarDetailHasNoSettingsOrBotName(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withAvatarPacks)
	f.openEngine("mfers")
	f.wantShown(locales.PluginsDetailInfo(), locales.PluginsStateReady(), locales.PluginsActionDisable(), locales.PluginsActionUninstall())
	f.wantHidden(locales.PluginsDetailSettingsHintEngine(), locales.PluginsDetailBotName(), locales.PluginsDetailSave(), locales.PluginsDetailReset(), locales.PluginsSetupStart())
}

func TestAvatarUninstallDialogMentionsInitials(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withAvatarPacks)
	f.installedTab()
	f.click(locales.PluginsActionMoreFor("mfers"))
	f.click(locales.PluginsActionUninstall())
	f.wantShown(locales.PluginsUninstallDialogTitle("mfers"), locales.PluginsUninstallDialogDescriptionAvatar())
	f.wantHidden(locales.PluginsUninstallDialogDescription(), locales.PluginsUninstallDialogDescriptionEngine())
}

func TestInstallAvatarPluginShowsItsHint(t *testing.T) {
	f := open(t, wideW, settings.SectionPlugins, withAvatarPacks)
	f.click(locales.PluginsInstallDialogOpen())
	f.typeInto(locales.PluginsInstallDialogSpec(), avatarPkg+"robots@1.0.0")
	f.click(locales.PluginsInstallDialogSubmit())
	f.Fake.FinishOperation(f.lastOperation().ID, true, "")
	f.poll(operationsAPI)
	f.awaitShown(locales.PluginsInstallDialogView())
	f.wantShown(locales.PluginsInstallDialogDoneHintAvatar())
	f.wantHidden(locales.PluginsInstallDialogDoneHint(), locales.PluginsInstallDialogDoneHintEngine(), locales.PluginsInstallDialogConfigure())
}
