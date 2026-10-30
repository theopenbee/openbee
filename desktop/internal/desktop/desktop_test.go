package desktop_test

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/theopenbee/openbee/desktop/internal/desktop"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	settingsAccelerator  = "CmdOrCtrl+,"
	newWorkerAccelerator = "CmdOrCtrl+N"
)

func find(items []*mygo.MenuItem, match func(*mygo.MenuItem) bool) *mygo.MenuItem {
	for _, it := range items {
		if match(it) {
			return it
		}
		if found := find(it.Submenu, match); found != nil {
			return found
		}
	}
	return nil
}

func top(items []*mygo.MenuItem, match func(*mygo.MenuItem) bool) *mygo.MenuItem {
	for _, it := range items {
		if match(it) {
			return it
		}
	}
	return nil
}

func accelerator(acc string) func(*mygo.MenuItem) bool {
	return func(it *mygo.MenuItem) bool { return it.Accelerator == acc }
}

func labeled(label string) func(*mygo.MenuItem) bool {
	return func(it *mygo.MenuItem) bool { return it.Label == label }
}

func role(r mygo.MenuRole) func(*mygo.MenuItem) bool {
	return func(it *mygo.MenuItem) bool { return it.Role == r }
}

func TestMenuDarwinHasSettingsAccelerator(t *testing.T) {
	items := desktop.Menu(desktop.Actions{}, "darwin")
	app := top(items, role(mygo.RoleAppMenu))
	if app == nil {
		t.Fatal("no app menu on darwin")
	}
	settings := find(app.Submenu, accelerator(settingsAccelerator))
	if settings == nil || settings.Label != locales.MenuSettings() {
		t.Fatalf("app menu settings item = %+v, want %q with %s", settings, locales.MenuSettings(), settingsAccelerator)
	}
	if find(app.Submenu, labeled(locales.MenuChangeServer())) == nil {
		t.Fatalf("app menu has no %q item", locales.MenuChangeServer())
	}
	for _, r := range []mygo.MenuRole{mygo.RoleAbout, mygo.RoleServices, mygo.RoleHide, mygo.RoleHideOthers, mygo.RoleUnhide, mygo.RoleQuit} {
		if find(app.Submenu, role(r)) == nil {
			t.Errorf("app menu has no %s role", r)
		}
	}
	file := top(items, labeled(locales.MenuFile()))
	if file == nil {
		t.Fatal("no File menu on darwin")
	}
	worker := find(file.Submenu, accelerator(newWorkerAccelerator))
	if worker == nil || worker.Label != locales.MenuNewWorker() {
		t.Fatalf("File new worker item = %+v, want %q with %s", worker, locales.MenuNewWorker(), newWorkerAccelerator)
	}
	if find(file.Submenu, role(mygo.RoleClose)) == nil {
		t.Error("File menu has no close role")
	}
	for _, r := range []mygo.MenuRole{mygo.RoleEditMenu, mygo.RoleViewMenu, mygo.RoleWindowMenu} {
		if top(items, role(r)) == nil {
			t.Errorf("no %s menu", r)
		}
	}
}

func TestMenuWindowsNoAppMenu(t *testing.T) {
	items := desktop.Menu(desktop.Actions{}, "windows")
	if find(items, role(mygo.RoleAppMenu)) != nil {
		t.Fatal("windows menu has an app menu")
	}
	file := top(items, labeled(locales.MenuFile()))
	if file == nil {
		t.Fatal("no File menu on windows")
	}
	for _, acc := range []string{settingsAccelerator, newWorkerAccelerator} {
		if find(file.Submenu, accelerator(acc)) == nil {
			t.Errorf("File menu has no %s item", acc)
		}
	}
	if find(file.Submenu, labeled(locales.MenuChangeServer())) == nil {
		t.Errorf("File menu has no %q item", locales.MenuChangeServer())
	}
	if find(file.Submenu, role(mygo.RoleQuit)) == nil {
		t.Error("File menu has no quit role")
	}
}

func TestMenuClickInvokesAction(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		var calls []string
		actions := desktop.Actions{
			Settings:     func() { calls = append(calls, "settings") },
			ChangeServer: func() { calls = append(calls, "change server") },
			NewWorker:    func() { calls = append(calls, "new worker") },
		}
		items := desktop.Menu(actions, goos)
		for _, tc := range []struct {
			label, want string
		}{
			{locales.MenuSettings(), "settings"},
			{locales.MenuChangeServer(), "change server"},
			{locales.MenuNewWorker(), "new worker"},
		} {
			calls = nil
			item := find(items, labeled(tc.label))
			if item == nil || item.Click == nil {
				t.Fatalf("%s: item %q = %+v, want a clickable item", goos, tc.label, item)
			}
			item.Click(item, nil)
			if len(calls) != 1 || calls[0] != tc.want {
				t.Errorf("%s: clicking %q called %v, want [%s]", goos, tc.label, calls, tc.want)
			}
		}
	}
}

func TestMenuInspectorOnlyInDebug(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		view := top(desktop.Menu(desktop.Actions{}, goos), role(mygo.RoleViewMenu))
		if view == nil || find(view.Submenu, role(mygo.RoleToggleFullScreen)) == nil {
			t.Fatalf("%s: view menu = %+v, want a full screen toggle", goos, view)
		}
		if find(view.Submenu, role(mygo.RoleToggleDevTools)) != nil {
			t.Errorf("%s: inspector shown without debug", goos)
		}
		debug := top(desktop.Menu(desktop.Actions{Inspector: true}, goos), role(mygo.RoleViewMenu))
		if find(debug.Submenu, role(mygo.RoleToggleDevTools)) == nil {
			t.Errorf("%s: no inspector in debug", goos)
		}
	}
}

func TestTitleWithUnread(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want string
	}{
		{-1, "OpenBee"},
		{0, "OpenBee"},
		{3, "(3) OpenBee"},
		{99, "(99) OpenBee"},
		{200, "(99+) OpenBee"},
	} {
		if got := desktop.TitleWithUnread(tc.n); got != tc.want {
			t.Errorf("TitleWithUnread(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestTrayMenu(t *testing.T) {
	opened := 0
	items := desktop.TrayMenu(func() { opened++ })
	if len(items) == 0 || items[0].Label != locales.TrayOpen() || items[0].Click == nil {
		t.Fatalf("first tray item = %+v, want %q", items, locales.TrayOpen())
	}
	items[0].Click(items[0], nil)
	if opened != 1 {
		t.Errorf("clicking %q opened %d times, want 1", locales.TrayOpen(), opened)
	}
	if top(items, role(mygo.RoleQuit)) == nil {
		t.Error("tray menu has no quit role")
	}
}

func TestTrayToolTip(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want string
	}{
		{0, "OpenBee"},
		{-2, "OpenBee"},
		{3, locales.TrayUnread(3)},
		{250, locales.TrayUnread(250)},
	} {
		if got := desktop.TrayToolTip(tc.n); got != tc.want {
			t.Errorf("TrayToolTip(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
