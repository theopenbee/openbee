package settings_test

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/internal/views/settings"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	appVersion = "1.0.6"
	versionAPI = "/api/version"
)

func openAbout(t *testing.T, u store.Updater, server api.ServerVersion) *fixture {
	t.Helper()
	hs := testkit.SignedInWith(t, wideW, wideH, func(st *fakeapi.State) { st.Version = server }, testkit.WithUpdater(u, appVersion))
	f := &fixture{Harness: hs, t: t}
	hs.Root.OpenSettings(settings.SectionAbout)
	f.settle()
	return f
}

func (f *fixture) wantOpened(url string) {
	f.t.Helper()
	if !slices.Contains(f.T.OpenedURLs(), url) {
		f.t.Fatalf("opened %q, want %q", f.T.OpenedURLs(), url)
	}
}

func TestAboutShowsVersions(t *testing.T) {
	f := openAbout(t, nil, api.ServerVersion{Version: "1.0.5"})
	f.wantPath(route.Settings(settings.SectionAbout, ""))
	f.wantShown(locales.AboutTitle(), locales.AboutSubtitle(), locales.AboutDesktopTitle(), locales.AboutDesktopDescription(), appVersion,
		locales.AboutServerTitle(), locales.AboutServerDescription(), "1.0.5",
		locales.AboutServerUpgrade(), locales.AboutServerUpgradeDescription(), settings.UpgradeCommand,
		locales.AboutReleasesTitle(), locales.AboutReleasesLink())
	if n := len(f.Fake.Calls(http.MethodGet, versionAPI)); n != 1 {
		t.Fatalf("version fetched %d times, want 1", n)
	}
}

func TestAboutWithoutSelfUpdateLinksToDownloads(t *testing.T) {
	f := openAbout(t, nil, api.ServerVersion{Version: "1.0.5"})
	f.wantShown(locales.AboutManualUpdates(), locales.AboutDownloadLatest())
	f.wantHidden(locales.AboutCheckUpdates(), locales.AboutAutoCheck(), locales.AboutAutoInstall())
	f.click(locales.AboutDownloadLatest())
	f.wantOpened(settings.LatestReleaseURL)
	f.clickIn(locales.AboutReleasesLink())
	f.wantOpened(settings.ReleasesURL)
}

func TestAboutCopiesUpgradeCommand(t *testing.T) {
	f := openAbout(t, nil, api.ServerVersion{Version: "1.0.5"})
	f.T.SetClipboard("")
	f.clickIn(locales.AboutServerCopyCommand())
	if got := f.T.Clipboard(); got != settings.UpgradeCommand {
		t.Fatalf("clipboard = %q, want %q", got, settings.UpgradeCommand)
	}
}

func TestAboutBundledServerHidesUpgrade(t *testing.T) {
	f := openAbout(t, nil, api.ServerVersion{Version: appVersion, Bundled: true})
	f.wantShown(locales.AboutServerBundledDescription())
	f.wantHidden(locales.AboutServerDescription(), locales.AboutServerUpgrade(), settings.UpgradeCommand)
}

func TestAboutOlderServerVersionIsUnknown(t *testing.T) {
	f := openAbout(t, nil, api.ServerVersion{})
	f.wantShown(locales.AboutVersionUnknown(), locales.AboutServerUpgrade(), settings.UpgradeCommand)
	f.wantHidden(locales.AboutLoadError())
}

func TestAboutVersionErrorRetries(t *testing.T) {
	hs := testkit.SignedIn(t, wideW, wideH, nil)
	f := &fixture{Harness: hs, t: t}
	f.Fake.FailNext(http.MethodGet, versionAPI, http.StatusInternalServerError, map[string]string{"error": "boom"})
	hs.Root.OpenSettings(settings.SectionAbout)
	f.settle()
	f.wantShown(locales.AboutLoadError(), locales.AboutDesktopTitle())
	f.click(locales.CommonRetry())
	f.wantHidden(locales.AboutLoadError())
	f.wantShown("1.0.6")
}

func TestAboutChecksForUpdates(t *testing.T) {
	u := &testkit.Updater{Automatic: true}
	f := openAbout(t, u, api.ServerVersion{Version: appVersion, Bundled: true})
	f.wantShown(locales.AboutUpdates(), locales.AboutNeverChecked(), locales.AboutCheckUpdates(),
		locales.AboutAutoCheck(), locales.AboutAutoCheckDescription(), locales.AboutAutoInstall(), locales.AboutAutoInstallDescription())
	f.wantHidden(locales.AboutManualUpdates(), locales.AboutDownloadLatest())
	f.click(locales.AboutCheckUpdates())
	if u.Checks != 1 {
		t.Fatalf("checks = %d, want 1", u.Checks)
	}
	u.Last = f.T.Now().Add(-3 * time.Hour)
	f.Root.OpenSettings(settings.SectionEnv)
	f.settle()
	f.Root.OpenSettings(settings.SectionAbout)
	f.settle()
	f.wantShown(locales.AboutLastChecked(locales.TimeHoursAgo(3)))
}

func TestAboutAutomaticUpdateSwitches(t *testing.T) {
	u := &testkit.Updater{Automatic: true}
	f := openAbout(t, u, api.ServerVersion{Version: appVersion, Bundled: true})
	f.clickIn(locales.AboutAutoInstall())
	if !u.Installing {
		t.Fatal("automatic install stayed off")
	}
	f.clickIn(locales.AboutAutoCheck())
	if u.Automatic {
		t.Fatal("automatic checks stayed on")
	}
	f.clickIn(locales.AboutAutoInstall())
	if !u.Installing {
		t.Fatal("automatic install changed while automatic checks are off")
	}
	f.clickIn(locales.AboutAutoCheck())
	f.clickIn(locales.AboutAutoInstall())
	if !u.Automatic || u.Installing {
		t.Fatalf("automatic=%v installing=%v, want checks on and install off", u.Automatic, u.Installing)
	}
}
