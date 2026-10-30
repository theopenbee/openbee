package theme

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLightTokens(t *testing.T) {
	l := Light()
	if l.Dark {
		t.Error("Light().Dark = true")
	}
	if l.Foreground != ui.Oklch(0.15, 0, 0) {
		t.Errorf("Light().Foreground = %+v", l.Foreground)
	}
	if l.Background != ui.Oklch(1, 0, 0) {
		t.Errorf("Light().Background = %+v", l.Background)
	}
	if l.TextSecondary != ui.Oklch(0.46, 0, 0) {
		t.Errorf("Light().TextSecondary = %+v", l.TextSecondary)
	}
	if l.Danger != ui.Oklch(0.54, 0.21, 25) {
		t.Errorf("Light().Danger = %+v", l.Danger)
	}
}

func TestDarkTokens(t *testing.T) {
	d := Dark()
	if !d.Dark {
		t.Error("Dark().Dark = false")
	}
	if d.Background != ui.Oklch(0.19, 0, 0) {
		t.Errorf("Dark().Background = %+v", d.Background)
	}
	if d.Foreground != ui.Oklch(0.96, 0, 0) {
		t.Errorf("Dark().Foreground = %+v", d.Foreground)
	}
	if d.Success != ui.Oklch(0.78, 0.18, 150) {
		t.Errorf("Dark().Success = %+v", d.Success)
	}
}

func TestAvatarPalette(t *testing.T) {
	if got := Light().Avatars[0]; got != ui.Oklch(0.72, 0.11, 25) {
		t.Errorf("Light().Avatars[0] = %+v", got)
	}
	if got := Dark().Avatars[7]; got != ui.Oklch(0.58, 0.11, 330) {
		t.Errorf("Dark().Avatars[7] = %+v", got)
	}
}

func TestConcentric(t *testing.T) {
	cases := []struct{ outer, padding, want float32 }{
		{34, 24, 10},
		{18, 16, 6},
		{26, 8, 18},
	}
	for _, c := range cases {
		if got := Concentric(c.outer, c.padding); got != c.want {
			t.Errorf("Concentric(%v, %v) = %v, want %v", c.outer, c.padding, got, c.want)
		}
	}
}

func TestApplySetsTheme(t *testing.T) {
	var tk Tokens
	var applied ui.Theme
	view := func(c *ui.Context) {
		tk = Apply(c)
		applied = *c.Theme()
		ui.Text(c, "probe")
	}
	tt := ui.NewTester(view, 200, 100)
	if tk.Dark {
		t.Error("Apply returned dark tokens in the light tester")
	}
	if applied.Background != tk.Background {
		t.Errorf("theme background = %+v, want %+v", applied.Background, tk.Background)
	}
	if applied.Radius != RadiusControl {
		t.Errorf("theme radius = %v, want %v", applied.Radius, RadiusControl)
	}
	if !strings.HasPrefix(applied.Font, FontBody) {
		t.Errorf("theme font = %q, want prefix %q", applied.Font, FontBody)
	}
	if applied.Accent != tk.ControlOn {
		t.Errorf("theme accent = %+v, want %+v", applied.Accent, tk.ControlOn)
	}
	tt.SetDark(true)
	tt.Frame()
	if !tk.Dark {
		t.Error("Apply returned light tokens after SetDark(true)")
	}
	if applied.Background != tk.Background || applied.Background != Dark().Background {
		t.Errorf("dark theme background = %+v, want %+v", applied.Background, Dark().Background)
	}
}

func TestApplySetsBodyLineHeight(t *testing.T) {
	view := func(c *ui.Context) {
		Apply(c)
		ui.Column(c).Padding(8).Children(func() {
			ui.Text(c, "probe")
			ui.Text(c, "tight").LineHeight(1)
		})
	}
	tt := ui.NewTester(view, 200, 100)
	body, ok := tt.Find("probe")
	if !ok {
		t.Fatal("probe text not found")
	}
	tight, ok := tt.Find("tight")
	if !ok {
		t.Fatal("tight text not found")
	}
	if want := baseFontSize * BodyLineHeight; body.H < want-0.5 || body.H > want+0.5 {
		t.Errorf("body text height = %v, want %v", body.H, want)
	}
	if tight.H >= body.H {
		t.Errorf("text with its own line height is %v high, want less than %v", tight.H, body.H)
	}
}

func TestRegisterFonts(t *testing.T) {
	if err := RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	registered := map[string]bool{}
	for _, f := range ui.FontFamilies() {
		if f.Registered {
			registered[f.Name] = true
		}
	}
	for _, name := range []string{FontBody, FontDisplay, FontMono} {
		if !registered[name] {
			t.Errorf("font family %q is not registered; registered: %v", name, registered)
		}
	}
}
