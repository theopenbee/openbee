package kit_test

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

func TestMain(m *testing.M) {
	if err := theme.RegisterFonts(); err != nil {
		fmt.Fprintln(os.Stderr, "register fonts:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func newTester(w, h int, build func(c *ui.Context, tk theme.Tokens)) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		tk := theme.Apply(c)
		ui.Column(c).Fill().Padding(24).Gap(12).AlignItems(ui.Start).Children(func() { build(c, tk) })
	}, w, h)
}

func settle(tt *ui.Tester) {
	tt.Advance(time.Second)
	tt.Frame()
}

func TestButtonClick(t *testing.T) {
	clicks := 0
	var opts kit.ButtonOpts
	tt := newTester(400, 200, func(c *ui.Context, tk theme.Tokens) {
		kit.Button(c, tk, "Save", opts).OnClick(func() { clicks++ })
	})
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	if clicks != 1 {
		t.Fatalf("clicks after one click = %d, want 1", clicks)
	}
	tt.Key(0, ui.KeyEnter)
	if clicks != 2 {
		t.Fatalf("clicks after Enter on the focused button = %d, want 2", clicks)
	}
	settle(tt)
	if tt.FramePending() {
		t.Error("an idle button keeps asking for frames")
	}

	opts.Disabled = true
	tt.Frame()
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	if clicks != 2 {
		t.Errorf("disabled button fired: clicks = %d, want 2", clicks)
	}

	opts.Disabled, opts.Loading = false, true
	tt.Frame()
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	if !tt.Focused("Save") {
		t.Fatal("loading button does not keep the keyboard focus")
	}
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeySpace)
	if clicks != 2 {
		t.Errorf("loading button fired: clicks = %d, want 2", clicks)
	}
	settle(tt)
	tt.Advance(100 * time.Millisecond)
	if !tt.FramePending() {
		t.Error("loading button shows no turning spinner")
	}
}

func TestIconButtonHasLabel(t *testing.T) {
	clicks := 0
	tt := newTester(300, 160, func(c *ui.Context, tk theme.Tokens) {
		kit.IconButton(c, tk, "plus", "New worker", kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).OnClick(func() { clicks++ })
	})
	r, ok := tt.Find("New worker")
	if !ok {
		t.Fatal("icon button not found by its label")
	}
	if r.W != theme.ControlSM || r.H != theme.ControlSM {
		t.Errorf("small icon button is %vx%v, want %vx%v", r.W, r.H, theme.ControlSM, theme.ControlSM)
	}
	if err := tt.Click("New worker"); err != nil {
		t.Fatal(err)
	}
	if clicks != 1 {
		t.Errorf("clicks = %d, want 1", clicks)
	}
}

func TestCopyIconButtonCopies(t *testing.T) {
	tt := newTester(300, 160, func(c *ui.Context, tk theme.Tokens) {
		kit.CopyIconButton(c, tk, "hello there", "Copy message")
	})
	r, ok := tt.Find("Copy message")
	if !ok {
		t.Fatal("copy button not found by its label")
	}
	if r.W != theme.ControlSM || r.H != theme.ControlSM {
		t.Errorf("copy button is %vx%v, want %vx%v", r.W, r.H, theme.ControlSM, theme.ControlSM)
	}
	if err := tt.Click("Copy message"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Clipboard(); got != "hello there" {
		t.Errorf("clipboard = %q", got)
	}
	if got := tt.Announcements(); len(got) != 1 || got[0] != locales.ArcCopyButtonCopiedAnnouncement("Copy message") {
		t.Errorf("announcements = %q", got)
	}
}

func TestPasswordToggle(t *testing.T) {
	value, shown := "hunter2", false
	tt := newTester(400, 200, func(c *ui.Context, tk theme.Tokens) {
		kit.PasswordInput(c, tk, &value, &shown, "Password", "Enter password")
	})
	if _, ok := tt.Find("Password"); !ok {
		t.Fatal("password input not found by its label")
	}
	if err := tt.Click(locales.ArcPasswordFieldShow()); err != nil {
		t.Fatal(err)
	}
	if !shown {
		t.Fatal("show button did not reveal the password")
	}
	if err := tt.Click(locales.ArcPasswordFieldHide()); err != nil {
		t.Fatal(err)
	}
	if shown {
		t.Error("hide button did not hide the password")
	}
}

func TestFieldShowsError(t *testing.T) {
	value := ""
	errText := ""
	tt := newTester(400, 240, func(c *ui.Context, tk theme.Tokens) {
		kit.Field(c, tk, "Username", "Letters and digits only", errText, func() {
			kit.TextInput(c, tk, &value, "Username", "admin")
		})
	})
	if !tt.HasText("Letters and digits only") {
		t.Error("help text missing")
	}
	if tt.HasText("Username is required") {
		t.Error("error shown while valid")
	}
	errText = "Username is required"
	tt.Frame()
	if !tt.HasText("Username is required") {
		t.Error("error text missing")
	}
	if err := tt.Click("Username"); err != nil {
		t.Fatal(err)
	}
	tt.Type("ada")
	if value != "ada" {
		t.Errorf("typed into the field's input: value = %q, want %q", value, "ada")
	}
}

func TestAlertDismiss(t *testing.T) {
	dismissed := 0
	tt := newTester(500, 200, func(c *ui.Context, tk theme.Tokens) {
		kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneInfo, Title: "Turn on notifications", Body: "Get a ping on replies.", OnDismiss: func() { dismissed++ }})
	})
	if !tt.HasText("Turn on notifications") || !tt.HasText("Get a ping on replies.") {
		t.Fatal("alert texts missing")
	}
	if err := tt.Click(locales.ArcAlertDismiss("Turn on notifications")); err != nil {
		t.Fatal(err)
	}
	if dismissed != 1 {
		t.Errorf("OnDismiss calls = %d, want 1", dismissed)
	}
}

func TestAlertAction(t *testing.T) {
	retried := 0
	tt := newTester(500, 220, func(c *ui.Context, tk theme.Tokens) {
		kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: "Could not load variables", Body: "connection refused", ActionLabel: "Retry", OnAction: func() { retried++ }})
	})
	if _, ok := tt.Find(locales.ArcAlertDismiss("Could not load variables")); ok {
		t.Error("alert without OnDismiss shows a dismiss button")
	}
	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	if retried != 1 {
		t.Errorf("OnAction calls = %d, want 1", retried)
	}
}

func TestEmptyStateAction(t *testing.T) {
	created := 0
	tt := newTester(500, 360, func(c *ui.Context, tk theme.Tokens) {
		kit.EmptyState(c, tk, "bot", "No workers yet", "Create a worker to start chatting.", func() {
			kit.Button(c, tk, "New worker", kit.ButtonOpts{Icon: "plus"}).OnClick(func() { created++ })
		})
	})
	if !tt.HasText("No workers yet") || !tt.HasText("Create a worker to start chatting.") {
		t.Fatal("empty state texts missing")
	}
	if err := tt.Click("New worker"); err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Errorf("action clicks = %d, want 1", created)
	}
}

type gallery struct {
	name, password, notes, invalid string
	shown                          bool
}

func (g *gallery) view(c *ui.Context) {
	tk := theme.Apply(c)
	ui.Row(c).Fill().Padding(32).Gap(40).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Width(640).Gap(20).Children(func() { g.controls(c, tk) })
		ui.Column(c).Grow(1).MinWidth(0).Gap(20).Children(func() { g.feedback(c, tk) })
	})
}

func (g *gallery) controls(c *ui.Context, tk theme.Tokens) {
	variants := []struct {
		name string
		v    kit.Variant
	}{{"Primary", kit.Primary}, {"Secondary", kit.Secondary}, {"Ghost", kit.Ghost}, {"Danger", kit.Danger}}
	for _, v := range variants {
		ui.Row(c).Gap(12).Wrap().Children(func() {
			kit.Button(c, tk, v.name, kit.ButtonOpts{Variant: v.v, Size: kit.SizeSM})
			kit.Button(c, tk, v.name, kit.ButtonOpts{Variant: v.v, Icon: "plus"})
			kit.Button(c, tk, v.name, kit.ButtonOpts{Variant: v.v, Size: kit.SizeLG})
			kit.Button(c, tk, v.name, kit.ButtonOpts{Variant: v.v, Loading: true})
			kit.Button(c, tk, v.name, kit.ButtonOpts{Variant: v.v, Disabled: true})
			kit.IconButton(c, tk, "settings", v.name+" settings", kit.ButtonOpts{Variant: v.v, Size: kit.SizeSM})
		})
	}
	kit.Field(c, tk, "Display name", "Shown in chats.", "", func() {
		kit.TextInput(c, tk, &g.name, "Display name", "Ada Lovelace")
	})
	kit.Field(c, tk, "Password", "", "", func() {
		kit.PasswordInput(c, tk, &g.password, &g.shown, "Password", "Enter password")
	})
	kit.Field(c, tk, "Server", "", "Enter a valid URL.", func() {
		kit.TextInput(c, tk, &g.invalid, "Server", "https://openbee.example.com")
	})
	kit.Field(c, tk, "Notes", "", "", func() {
		kit.TextArea(c, tk, &g.notes, "Notes", "Anything the worker should know", 3, 6)
	})
}

func (g *gallery) feedback(c *ui.Context, tk theme.Tokens) {
	kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneInfo, Title: "Turn on notifications", Body: "Get a desktop notification when a worker replies.", OnDismiss: func() {}})
	kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneSuccess, Title: "Password changed"})
	kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneWarning, Title: "Connection lost", Body: "Reconnecting to the server."})
	kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: "Could not load variables", Body: "dial tcp 127.0.0.1:8080: connection refused", ActionLabel: "Retry", OnAction: func() {}})
	ui.Row(c).Gap(8).Children(func() {
		kit.Badge(c, tk, "Idle", kit.ToneNeutral)
		kit.Badge(c, tk, "3 set", kit.ToneInfo)
		kit.Badge(c, tk, "Running", kit.ToneSuccess)
		kit.Badge(c, tk, "Retrying", kit.ToneWarning)
		kit.Badge(c, tk, "Failed", kit.ToneDanger)
	})
	ui.Row(c).Gap(12).Children(func() {
		kit.Spinner(c, tk, 16)
		kit.Shimmer(c, tk, "Thinking about your request")
	})
	ui.Row(c).Gap(16).Children(func() {
		kit.SkeletonCircle(c, tk, theme.ControlMD)
		ui.Column(c).Grow(1).Gap(12).Children(func() {
			kit.SkeletonBar(c, tk, 160, 16)
			kit.SkeletonBar(c, tk, 0, 12)
			kit.SkeletonBar(c, tk, 240, 12)
		})
	})
	kit.Panel(c, tk, "Installed plugins", "Channels and tools running on this server.", func() {
		kit.Button(c, tk, "Install", kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM, Icon: "plus"})
	}, func() {
		kit.EmptyState(c, tk, "puzzle", "No plugins yet", "Install a plugin to connect a channel.", func() {
			kit.Button(c, tk, "Browse plugins", kit.ButtonOpts{Variant: kit.Secondary})
		})
	})
}

func TestKitGallery(t *testing.T) {
	g := &gallery{name: "Ada Lovelace", password: "hunter2", invalid: "openbee"}
	tt := ui.NewTester(g.view, 1280, 1040)
	tt.SetScale(2)
	for _, text := range []string{"Primary", "Display name", "Turn on notifications", "Running", "Installed plugins", "No plugins yet", "Thinking about your request"} {
		if !tt.HasText(text) {
			t.Errorf("gallery is missing %q", text)
		}
	}
	shoot(t, tt, "kit-gallery", nil)
	for _, s := range overlayShots() {
		tt := ui.NewTester(s.view, s.w, s.h)
		tt.SetScale(2)
		for _, text := range s.texts {
			if !tt.HasText(text) {
				t.Errorf("%s is missing %q", s.name, text)
			}
		}
		shoot(t, tt, s.name, s.prepare)
	}
}

func shoot(t *testing.T, tt *ui.Tester, name string, prepare func(t *testing.T, tt *ui.Tester)) {
	t.Helper()
	dir := os.Getenv("OPENBEE_SCREENS_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"light", "dark"} {
		tt.SetDark(mode == "dark")
		tt.Frame()
		if prepare != nil {
			prepare(t, tt)
		}
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		writePNG(t, tt, filepath.Join(dir, name+"-"+mode+".png"))
	}
}

func writePNG(t *testing.T, tt *ui.Tester, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
