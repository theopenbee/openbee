package desktop

import (
	"slices"

	"github.com/egoist/mygo"
	"github.com/theopenbee/openbee/desktop/locales"
)

const darwin = "darwin"

type Actions struct {
	Settings, ChangeServer, NewWorker, CheckUpdates func()
	Inspector                                       bool
}

func Menu(a Actions, goos string) []*mygo.MenuItem {
	settings := &mygo.MenuItem{Label: locales.MenuSettings(), Accelerator: "CmdOrCtrl+,", Click: click(a.Settings)}
	changeServer := &mygo.MenuItem{Label: locales.MenuChangeServer(), Click: click(a.ChangeServer)}
	newWorker := &mygo.MenuItem{Label: locales.MenuNewWorker(), Accelerator: "CmdOrCtrl+N", Click: click(a.NewWorker)}
	var updates []*mygo.MenuItem
	if a.CheckUpdates != nil {
		updates = []*mygo.MenuItem{{Label: locales.MenuCheckUpdates(), Click: click(a.CheckUpdates)}}
	}
	view := []*mygo.MenuItem{{Role: mygo.RoleToggleFullScreen}}
	if a.Inspector {
		view = append(view, mygo.Separator(), &mygo.MenuItem{Role: mygo.RoleToggleDevTools})
	}
	standard := []*mygo.MenuItem{
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleViewMenu, Submenu: view},
		{Role: mygo.RoleWindowMenu},
	}
	if goos != darwin {
		file := &mygo.MenuItem{Label: locales.MenuFile(), Submenu: slices.Concat(
			[]*mygo.MenuItem{newWorker, mygo.Separator(), settings, changeServer},
			updates,
			[]*mygo.MenuItem{mygo.Separator(), {Role: mygo.RoleQuit}},
		)}
		return append([]*mygo.MenuItem{file}, standard...)
	}
	app := &mygo.MenuItem{Role: mygo.RoleAppMenu, Submenu: slices.Concat(
		[]*mygo.MenuItem{{Role: mygo.RoleAbout}},
		updates,
		[]*mygo.MenuItem{
			mygo.Separator(),
			settings,
			changeServer,
			mygo.Separator(),
			{Role: mygo.RoleServices},
			mygo.Separator(),
			{Role: mygo.RoleHide},
			{Role: mygo.RoleHideOthers},
			{Role: mygo.RoleUnhide},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		},
	)}
	file := &mygo.MenuItem{Label: locales.MenuFile(), Submenu: []*mygo.MenuItem{
		newWorker,
		mygo.Separator(),
		{Role: mygo.RoleClose},
	}}
	return append([]*mygo.MenuItem{app, file}, standard...)
}

func click(fn func()) func(*mygo.MenuItem, *mygo.Window) {
	return func(*mygo.MenuItem, *mygo.Window) {
		if fn != nil {
			fn()
		}
	}
}
