package desktop_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo"
	"github.com/theopenbee/openbee/desktop/internal/desktop"
	"github.com/theopenbee/openbee/desktop/locales"
)

func TestMenuCheckUpdatesFollowsAbout(t *testing.T) {
	checked := 0
	items := desktop.Menu(desktop.Actions{CheckUpdates: func() { checked++ }}, "darwin")
	app := top(items, role(mygo.RoleAppMenu))
	i := slices.IndexFunc(app.Submenu, labeled(locales.MenuCheckUpdates()))
	if i != 1 || app.Submenu[0].Role != mygo.RoleAbout {
		t.Fatalf("check for updates at %d of the app menu, want right after About", i)
	}
	item := app.Submenu[i]
	item.Click(item, nil)
	if checked != 1 {
		t.Errorf("clicking check for updates called it %d times, want 1", checked)
	}
	if find(top(items, labeled(locales.MenuFile())).Submenu, labeled(locales.MenuCheckUpdates())) != nil {
		t.Error("darwin File menu has check for updates too")
	}
}

func TestMenuCheckUpdatesInFileMenu(t *testing.T) {
	for _, goos := range []string{"windows", "linux"} {
		checked := 0
		file := top(desktop.Menu(desktop.Actions{CheckUpdates: func() { checked++ }}, goos), labeled(locales.MenuFile()))
		i := slices.IndexFunc(file.Submenu, labeled(locales.MenuCheckUpdates()))
		if i < 1 || file.Submenu[i-1].Label != locales.MenuChangeServer() {
			t.Fatalf("%s: check for updates at %d of the File menu, want after change server", goos, i)
		}
		item := file.Submenu[i]
		item.Click(item, nil)
		if checked != 1 {
			t.Errorf("%s: clicking check for updates called it %d times, want 1", goos, checked)
		}
	}
}

func TestMenuWithoutUpdates(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		if find(desktop.Menu(desktop.Actions{}, goos), labeled(locales.MenuCheckUpdates())) != nil {
			t.Errorf("%s: check for updates shown for a build that cannot update", goos)
		}
	}
}

func TestSelfUpdates(t *testing.T) {
	installed := t.TempDir()
	if err := os.WriteFile(filepath.Join(installed, "Uninstall.exe"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	portable := t.TempDir()
	for _, tc := range []struct {
		goos, exe string
		want      bool
	}{
		{"darwin", "/Applications/OpenBee.app/Contents/MacOS/OpenBee", true},
		{"linux", "/home/me/.local/openbee-desktop.app/openbee-desktop", true},
		{"windows", filepath.Join(installed, "OpenBee.exe"), true},
		{"windows", filepath.Join(portable, "OpenBee.exe"), false},
		{"windows", filepath.Join(portable, "openbee-desktop-1.0.1-windows-amd64.exe"), false},
	} {
		if got := desktop.SelfUpdates(tc.goos, tc.exe); got != tc.want {
			t.Errorf("SelfUpdates(%s, %s) = %v, want %v", tc.goos, tc.exe, got, tc.want)
		}
	}
}
