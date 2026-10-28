package kit_test

import (
	"image/color"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

func distinctColorIDs(t *testing.T) (string, string) {
	t.Helper()
	first := "worker-0"
	for i := 1; i < 64; i++ {
		id := "worker-" + strconv.Itoa(i)
		if model.AvatarIndex(id) != model.AvatarIndex(first) {
			return first, id
		}
	}
	t.Fatal("no two ids with different avatar colours")
	return "", ""
}

func pixelAbove(t *testing.T, tt *ui.Tester, text string) color.RGBA {
	t.Helper()
	r, ok := tt.Find(text)
	if !ok {
		t.Fatalf("%q not found", text)
	}
	return tt.Image().RGBAAt(int(r.X+r.W/2), int(r.Y-4))
}

func near(got color.RGBA, want ui.Color) bool {
	diff := func(a, b uint8) int {
		if a > b {
			return int(a - b)
		}
		return int(b - a)
	}
	return diff(got.R, want.R) <= 3 && diff(got.G, want.G) <= 3 && diff(got.B, want.B) <= 3
}

func TestWorkerAvatarInitialsAndColor(t *testing.T) {
	first, second := distinctColorIDs(t)
	tt := newTester(400, 200, func(c *ui.Context, tk theme.Tokens) {
		ui.Row(c).Gap(16).Children(func() {
			kit.WorkerAvatar(c, tk, first, "Ada Lovelace", "idle", kit.AvatarXL, false)
			kit.WorkerAvatar(c, tk, second, "grace_hopper", "idle", kit.AvatarXL, false)
			kit.InitialsAvatar(c, tk, "Telegram", kit.AvatarXL)
		})
	})
	tk := theme.Light()
	a, b := pixelAbove(t, tt, "AL"), pixelAbove(t, tt, "GH")
	if !near(a, tk.Avatars[model.AvatarIndex(first)]) {
		t.Errorf("avatar of %q is %v, want %+v", first, a, tk.Avatars[model.AvatarIndex(first)])
	}
	if !near(b, tk.Avatars[model.AvatarIndex(second)]) {
		t.Errorf("avatar of %q is %v, want %+v", second, b, tk.Avatars[model.AvatarIndex(second)])
	}
	if a == b {
		t.Errorf("ids %q and %q render the same background %v", first, second, a)
	}
	if !tt.HasText("T") {
		t.Error("initials avatar does not show the first letter of the name")
	}
}

func TestStatusDotLabels(t *testing.T) {
	tt := newTester(400, 200, func(c *ui.Context, tk theme.Tokens) {
		ui.Row(c).Gap(16).Children(func() {
			kit.WorkerAvatar(c, tk, "a", "Ada", "working", kit.AvatarMD, true)
			kit.WorkerAvatar(c, tk, "b", "Bob", "paused", kit.AvatarMD, true)
			kit.WorkerAvatar(c, tk, "c", "Cy", "error", kit.AvatarMD, false)
		})
	})
	if _, ok := tt.Find(locales.StatusesWorking()); !ok {
		t.Error("working status dot has no label")
	}
	if _, ok := tt.Find(locales.StatusesError()); ok {
		t.Error("avatar without status shows a status dot")
	}
	tt.Advance(300 * time.Millisecond)
	if !tt.FramePending() {
		t.Error("working status dot does not ping")
	}
}

func TestDialogEscapeCloses(t *testing.T) {
	open := true
	tt := newTester(800, 600, func(c *ui.Context, tk theme.Tokens) {
		kit.Dialog(c, tk, &open, kit.DialogOpts{Title: "Rename worker", Description: "Pick a new name."}, func() {
			ui.Text(c, "Dialog body")
		})
	})
	if !tt.HasText("Rename worker") || !tt.HasText("Pick a new name.") || !tt.HasText("Dialog body") {
		t.Fatal("dialog texts missing")
	}
	tt.Key(0, ui.KeyEscape)
	if open {
		t.Fatal("Escape left the dialog open")
	}
	tt.Frame()
	if tt.HasText("Dialog body") {
		t.Error("closed dialog still shows")
	}
	open = true
	tt.Frame()
	if err := tt.Click(locales.ArcDialogClose()); err != nil {
		t.Fatal(err)
	}
	if open {
		t.Error("close button left the dialog open")
	}
}

func TestDialogFlushFillsTheFixedHeight(t *testing.T) {
	for _, tc := range []struct{ window, want int }{{700, 500}, {400, 368}} {
		open := true
		tt := newTester(900, tc.window, func(c *ui.Context, tk theme.Tokens) {
			kit.Dialog(c, tk, &open, kit.DialogOpts{Title: "Settings", Width: 600, Height: 500, Flush: true}, func() {
				ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
					ui.Column(c).Width(120).Children(func() { ui.Text(c, "Nav") })
					ui.Column(c).Grow(1).Children(func() { ui.Text(c, "Body") })
				})
			})
		})
		settle(tt)
		panel := mustFind(t, tt, "Settings")
		if got := panel.H; got < float32(tc.want)-1 || got > float32(tc.want)+1 {
			t.Errorf("window %d: panel height = %v, want %d", tc.window, got, tc.want)
		}
		nav, body := mustFind(t, tt, "Nav"), mustFind(t, tt, "Body")
		if nav.Y != body.Y || body.X < nav.X+nav.W-1 {
			t.Errorf("window %d: flush body is not laid out as a row: nav %+v, body %+v", tc.window, nav, body)
		}
		if nav.X-panel.X > 4 {
			t.Errorf("window %d: flush body is padded: nav starts %v from the panel edge", tc.window, nav.X-panel.X)
		}
	}
}

func mustFind(t *testing.T, tt *ui.Tester, text string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(text)
	if !ok {
		t.Fatalf("no %q; texts %q", text, tt.Texts())
	}
	return r
}

func TestDialogLockedIgnoresEscape(t *testing.T) {
	open := true
	tt := newTester(800, 600, func(c *ui.Context, tk theme.Tokens) {
		kit.Dialog(c, tk, &open, kit.DialogOpts{Title: "Saving", Locked: true}, func() {
			ui.Text(c, "Please wait")
		})
	})
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if !open || !tt.HasText("Please wait") {
		t.Fatal("Escape closed a locked dialog")
	}
	tt.ClickAt(10, 10)
	tt.Frame()
	if !open {
		t.Fatal("clicking outside closed a locked dialog")
	}
	if err := tt.Click(locales.ArcDialogClose()); err != nil {
		t.Fatal(err)
	}
	if !open {
		t.Error("close button closed a locked dialog")
	}
}

func TestDrawerOutsideClickCloses(t *testing.T) {
	open := true
	tt := newTester(900, 600, func(c *ui.Context, tk theme.Tokens) {
		kit.Drawer(c, tk, &open, "Worker", 320, func() {
			ui.Text(c, "Drawer body")
		})
	})
	settle(tt)
	if !tt.HasText("Worker") || !tt.HasText("Drawer body") {
		t.Fatal("drawer texts missing")
	}
	if r, _ := tt.Find("Worker"); r.X != 900-320 || r.W != 320 || r.H != 600 {
		t.Errorf("drawer panel = %+v, want the right 320 DIPs at full height", r)
	}
	tt.ClickAt(100, 300)
	if open {
		t.Fatal("clicking outside left the drawer open")
	}
	open = true
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	if open {
		t.Error("Escape left the drawer open")
	}
	open = true
	tt.SetSize(250, 600)
	settle(tt)
	if r, _ := tt.Find("Worker"); r.X != 0 || r.W != 250 {
		t.Errorf("drawer in a 250 DIP window = %+v, want the whole width", r)
	}
}

func TestSegmentedChanges(t *testing.T) {
	selected, changes := 0, 0
	items := []kit.SegmentItem{{Text: "Day"}, {Text: "Week"}, {Icon: "settings", Tooltip: "Custom"}}
	tt := newTester(500, 200, func(c *ui.Context, tk theme.Tokens) {
		if kit.Segmented(c, tk, &selected, "Period", items).Changed() {
			changes++
		}
	})
	if err := tt.Click("Week"); err != nil {
		t.Fatal(err)
	}
	if selected != 1 {
		t.Errorf("selected = %d, want 1", selected)
	}
	if err := tt.Click("Custom"); err != nil {
		t.Fatal(err)
	}
	if selected != 2 {
		t.Errorf("selected after the icon segment = %d, want 2", selected)
	}
	if changes != 2 {
		t.Errorf("changes = %d, want 2", changes)
	}
}

func TestTabsChange(t *testing.T) {
	selected := 0
	tt := newTester(500, 200, func(c *ui.Context, tk theme.Tokens) {
		kit.Tabs(c, tk, &selected, "Worker", []string{"Profile", "Edit", "Env"})
	})
	if err := tt.Click("Env"); err != nil {
		t.Fatal(err)
	}
	if selected != 2 {
		t.Errorf("selected = %d, want 2", selected)
	}
}

func TestCompactTabsFitTheirLabels(t *testing.T) {
	wide, compact := 0, 0
	items := []string{"Constraints", "Permissions", "Env"}
	tt := newTester(800, 300, func(c *ui.Context, tk theme.Tokens) {
		kit.Tabs(c, tk, &wide, "Wide tabs", items)
		kit.CompactTabs(c, tk, &compact, "Compact tabs", items)
	})
	settle(tt)
	wideBox, _ := tt.Find("Wide tabs")
	compactBox, ok := tt.Find("Compact tabs")
	if !ok || compactBox.W >= wideBox.W {
		t.Fatalf("compact tabs width = %v, want narrower than the regular %v", compactBox.W, wideBox.W)
	}
	tt.ClickAt(compactBox.X+compactBox.W-12, compactBox.Y+compactBox.H/2)
	if compact != 2 || wide != 0 {
		t.Errorf("selected = compact %d wide %d, want compact 2 wide 0", compact, wide)
	}
}

func TestSwitchAndCheckboxRows(t *testing.T) {
	sound, locked, wipe := false, true, false
	tt := newTester(500, 300, func(c *ui.Context, tk theme.Tokens) {
		kit.SwitchRow(c, tk, &sound, "Sound", "Play a sound on reply.", false)
		kit.SwitchRow(c, tk, &locked, "Locked", "", true)
		kit.CheckboxRow(c, tk, &wipe, "Delete work directory", "Removes every file.")
	})
	if err := tt.Click("Sound"); err != nil {
		t.Fatal(err)
	}
	if !sound {
		t.Error("switch row did not turn on")
	}
	if err := tt.Click("Locked"); err != nil {
		t.Fatal(err)
	}
	if !locked {
		t.Error("disabled switch row changed")
	}
	if err := tt.Click("Delete work directory"); err != nil {
		t.Fatal(err)
	}
	if !wipe {
		t.Error("checkbox row did not check")
	}
	if !tt.HasText("Removes every file.") {
		t.Error("checkbox description missing")
	}
}

func TestCopyButtonWritesClipboard(t *testing.T) {
	tt := newTester(400, 200, func(c *ui.Context, tk theme.Tokens) {
		kit.CopyButton(c, tk, "OPENBEE_TOKEN", "Copy key")
	})
	if err := tt.Click("Copy key"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Clipboard(); got != "OPENBEE_TOKEN" {
		t.Errorf("clipboard = %q, want %q", got, "OPENBEE_TOKEN")
	}
	tt.Frame()
	if !tt.HasText(locales.ArcCopyButtonCopied()) {
		t.Fatal("copy button does not show the copied label")
	}
	tt.Advance(2 * time.Second)
	tt.Frame()
	if tt.HasText(locales.ArcCopyButtonCopied()) {
		t.Error("copied label stays after the feedback time")
	}
}

func TestSelectChooses(t *testing.T) {
	engine, size := "claude", ""
	options := []kit.SelectOption{{Value: "claude", Text: "Claude Code"}, {Value: "codex", Text: "Codex"}, {Value: "pi", Text: "Pi"}}
	tt := newTester(500, 400, func(c *ui.Context, tk theme.Tokens) {
		kit.Select(c, tk, &engine, "Engine", options, "")
		kit.Select(c, tk, &size, "Size", options, "")
	})
	if !tt.HasText("Claude Code") {
		t.Fatal("select does not show the chosen option")
	}
	if !tt.HasText(locales.ArcSelectPlaceholder()) {
		t.Error("empty select does not show the placeholder")
	}
	if err := tt.Click("Engine"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Codex") {
		t.Fatal("popup does not list the options")
	}
	if err := tt.Click("Codex"); err != nil {
		t.Fatal(err)
	}
	if engine != "codex" {
		t.Errorf("engine = %q, want %q", engine, "codex")
	}
	tt.Frame()
	if tt.HasText("Pi") {
		t.Error("popup stays open after choosing")
	}
}

func TestShowToastsDrains(t *testing.T) {
	queue := []kit.Toast{{Tone: kit.ToneSuccess, Title: "Plugin installed", Description: "Telegram is ready."}}
	tt := newTester(800, 600, func(c *ui.Context, tk theme.Tokens) {
		kit.ShowToasts(c, tk, &queue)
	})
	if len(queue) != 0 {
		t.Fatalf("queue holds %d toasts after a frame, want 0", len(queue))
	}
	tt.Frame()
	if !tt.HasText("Plugin installed") || !tt.HasText("Telegram is ready.") {
		t.Fatal("toast texts missing")
	}
	if err := tt.Click(locales.ArcToastDismiss()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("Plugin installed") {
		t.Error("dismissed toast still shows")
	}
}

type overlayShot struct {
	name    string
	w, h    int
	view    func(c *ui.Context)
	texts   []string
	prepare func(t *testing.T, tt *ui.Tester)
}

type widgetGallery struct {
	period, theme, tab     int
	sound, multiline, lock bool
	wipe, keep             bool
	engine, size           string
	toasts                 []kit.Toast
}

var engineOptions = []kit.SelectOption{{Value: "claude", Text: "Claude Code"}, {Value: "codex", Text: "Codex"}, {Value: "pi", Text: "Pi"}}

func (g *widgetGallery) view(c *ui.Context) {
	tk := theme.Apply(c)
	ui.Row(c).Fill().Padding(32).Gap(48).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Width(560).Gap(24).Children(func() {
			ui.Row(c).Gap(20).Children(func() {
				kit.WorkerAvatar(c, tk, "w-1", "Ada Lovelace", "idle", kit.AvatarSM, true)
				kit.WorkerAvatar(c, tk, "w-2", "grace_hopper", "working", kit.AvatarMD, true)
				kit.WorkerAvatar(c, tk, "w-3", "Linus", "error", kit.AvatarLG, true)
				kit.WorkerAvatar(c, tk, "w-4", "张三", "paused", kit.AvatarXL, true)
				kit.WorkerAvatar(c, tk, "w-5", "Margaret Hamilton", "idle", kit.AvatarXL, false)
			})
			ui.Row(c).Gap(20).Children(func() {
				for _, size := range []kit.AvatarSize{kit.AvatarSM, kit.AvatarMD, kit.AvatarLG, kit.AvatarXL} {
					kit.BeeAvatar(c, tk, size)
				}
				kit.UnknownAvatar(c, tk, kit.AvatarMD)
				kit.InitialsAvatar(c, tk, "Telegram Bot", kit.AvatarMD)
				kit.InitialsAvatar(c, tk, "discord", kit.AvatarLG)
			})
			ui.Row(c).Gap(16).Children(func() {
				for _, status := range []string{"idle", "working", "error", "paused"} {
					kit.StatusDot(c, tk, status, 10)
				}
			})
			kit.Segmented(c, tk, &g.period, "Catalog", []kit.SegmentItem{{Text: "Market"}, {Text: "Installed"}, {Text: "Updates", Icon: "refresh-cw"}})
			kit.Segmented(c, tk, &g.theme, "Theme", []kit.SegmentItem{{Icon: "sun", Tooltip: "Light"}, {Icon: "moon", Tooltip: "Dark"}, {Icon: "monitor", Tooltip: "System"}})
			kit.Tabs(c, tk, &g.tab, "Worker", []string{"Profile", "Edit", "Env"})
			ui.Row(c).Gap(12).Children(func() {
				kit.CopyButton(c, tk, "OPENBEE_TOKEN", "")
				kit.CopyButton(c, tk, "OPENBEE_TOKEN", "Copy key")
			})
		})
		ui.Column(c).Width(520).Gap(16).Children(func() {
			kit.SwitchRow(c, tk, &g.sound, "Read files", "Let the worker read files in its work directory.", false)
			kit.SwitchRow(c, tk, &g.lock, "Run commands", "Needs an administrator.", true)
			kit.SwitchRow(c, tk, &g.multiline, "Multi-line value", "", false)
			kit.CheckboxRow(c, tk, &g.wipe, "Delete work directory", "Removes every file the worker wrote.")
			kit.CheckboxRow(c, tk, &g.keep, "Keep history", "")
			kit.Field(c, tk, "Engine", "", "", func() {
				kit.Select(c, tk, &g.engine, "Engine", engineOptions, "")
			})
			kit.Field(c, tk, "Model", "Leave empty for the default.", "", func() {
				kit.Select(c, tk, &g.size, "Model", engineOptions, "")
			})
		})
	})
	kit.ShowToasts(c, tk, &g.toasts)
}

type overlayGallery struct {
	dialog, drawer, popover bool
	engine, password, next  string
	shown, nextShown        bool
	tab                     int
}

func (g *overlayGallery) page(c *ui.Context, tk theme.Tokens) ui.Element {
	var anchor ui.Element
	ui.Column(c).Fill().Padding(32).Gap(16).AlignItems(ui.Start).Children(func() {
		ui.Text(c, "Workers").FontSize(22)
		ui.Text(c, "Everything behind the overlay stays visible through the scrim.").TextColor(tk.TextSecondary)
		ui.Row(c).Gap(12).Children(func() {
			anchor = kit.Button(c, tk, "Filters", kit.ButtonOpts{Variant: kit.Secondary, Icon: "sliders-horizontal"})
			kit.IconButton(c, tk, "settings", "Worker settings", kit.ButtonOpts{Variant: kit.Ghost})
			kit.Select(c, tk, &g.engine, "Engine", engineOptions, "").Width(240)
		})
		kit.Panel(c, tk, "Ada Lovelace", "Writes the release notes.", nil, func() {
			kit.Tabs(c, tk, &g.tab, "Worker", []string{"Profile", "Edit", "Env"})
		}).Width(560)
	})
	return anchor
}

func (g *overlayGallery) view(c *ui.Context) {
	tk := theme.Apply(c)
	anchor := g.page(c, tk)
	kit.Popover(c, tk, anchor, &g.popover, 240, func() {
		for _, item := range []string{"Unread only", "Working now", "Has errors"} {
			ui.Row(c).MinHeight(36).PaddingX(11).Children(func() { ui.Text(c, item) })
		}
	})
	kit.Dialog(c, tk, &g.dialog, kit.DialogOpts{
		Title:       "Change password",
		Description: "Changing it signs you out of every session.",
		Footer: func() {
			kit.Button(c, tk, "Cancel", kit.ButtonOpts{Variant: kit.Secondary})
			kit.Button(c, tk, "Change password", kit.ButtonOpts{})
		},
	}, func() {
		ui.Column(c).Gap(16).Children(func() {
			kit.Field(c, tk, "Current password", "", "", func() {
				kit.PasswordInput(c, tk, &g.password, &g.shown, "Current password", "")
			})
			kit.Field(c, tk, "New password", "Use at least 6 characters.", "", func() {
				kit.PasswordInput(c, tk, &g.next, &g.nextShown, "New password", "")
			})
		})
	})
	kit.Drawer(c, tk, &g.drawer, "Ada Lovelace", theme.DrawerMaxWidth, func() {
		ui.Column(c).Gap(16).AlignItems(ui.Start).Children(func() {
			kit.WorkerAvatar(c, tk, "w-1", "Ada Lovelace", "working", kit.AvatarXL, true)
			ui.Text(c, "Writes the release notes and keeps the changelog short.").TextColor(tk.TextSecondary)
			kit.Tabs(c, tk, &g.tab, "Drawer tabs", []string{"Profile", "Edit", "Env"})
		})
	})
}

func overlayShots() []overlayShot {
	widgets := &widgetGallery{theme: 2, sound: true, wipe: true, engine: "codex", toasts: []kit.Toast{
		{Tone: kit.ToneSuccess, Title: "Plugin installed", Description: "Telegram is ready to receive messages."},
		{Tone: kit.ToneDanger, Title: "Could not stop the task", Description: "connection refused"},
		{Tone: kit.ToneInfo, Title: "Notifications are on"},
	}}
	dialog := &overlayGallery{dialog: true, password: "hunter2"}
	drawer := &overlayGallery{drawer: true}
	popover := &overlayGallery{popover: true}
	selecting := &overlayGallery{engine: "claude"}
	return []overlayShot{
		{name: "kit-widgets", w: 1280, h: 860, view: widgets.view, texts: []string{"Market", "Profile", "Read files", "Codex", "Plugin installed"},
			prepare: func(t *testing.T, tt *ui.Tester) {
				if !tt.HasText(locales.ArcCopyButtonCopied()) {
					if err := tt.Click("Copy key"); err != nil {
						t.Fatal(err)
					}
				}
			}},
		{name: "kit-dialog", w: 1280, h: 800, view: dialog.view, texts: []string{"Change password", "Use at least 6 characters."}},
		{name: "kit-drawer", w: 1280, h: 800, view: drawer.view, texts: []string{"Ada Lovelace", "Drawer tabs"}},
		{name: "kit-popover", w: 1280, h: 800, view: popover.view, texts: []string{"Unread only"},
			prepare: func(t *testing.T, tt *ui.Tester) {
				r, ok := tt.Find("Worker settings")
				if !ok {
					t.Fatal("icon button not found")
				}
				tt.Move(r.X+r.W/2, r.Y+r.H/2)
				tt.Advance(time.Second)
			}},
		{name: "kit-select", w: 1280, h: 800, view: selecting.view, texts: []string{"Claude Code"},
			prepare: func(t *testing.T, tt *ui.Tester) {
				if !tt.HasText("Pi") {
					if err := tt.Click("Engine"); err != nil {
						t.Fatal(err)
					}
				}
			}},
	}
}
