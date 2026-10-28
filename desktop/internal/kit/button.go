package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

type Variant int

const (
	Primary Variant = iota
	Secondary
	Ghost
	Danger
)

type Size int

const (
	SizeSM Size = iota
	SizeMD
	SizeLG
)

type ButtonOpts struct {
	Variant  Variant
	Size     Size
	Icon     string
	Loading  bool
	Disabled bool
	Label    string
}

const (
	durationFast     = 160 * time.Millisecond
	bodyFontSize     = 14
	smallFontSize    = 12
	mediumWeight     = 500
	buttonGap        = 8
	buttonIconSize   = 16
	iconButtonGlyph  = 18
	buttonSpinPeriod = 700 * time.Millisecond
	primaryHover     = 0.91
	primaryPress     = 0.84
	pressScale       = 0.97
	iconPressScale   = 0.96
)

var (
	easeStandard = ui.CubicBezier(0.22, 1, 0.36, 1)
	fast         = ui.Animation{Duration: durationFast, Ease: easeStandard}
)

func (s Size) height() float32 {
	switch s {
	case SizeSM:
		return theme.ControlSM
	case SizeLG:
		return theme.ControlLG
	}
	return theme.ControlMD
}

func (s Size) paddingX() float32 {
	switch s {
	case SizeSM:
		return 12
	case SizeLG:
		return 20
	}
	return 16
}

type buttonLook struct {
	bg, fg, border ui.Color
	opacity        float32
	shadow         bool
}

func lookOf(tk theme.Tokens, v Variant, hovered, pressed bool) buttonLook {
	active := hovered || pressed
	switch v {
	case Secondary:
		l := buttonLook{bg: tk.Surface, fg: tk.Foreground, border: tk.Border, opacity: 1, shadow: hovered && !pressed}
		if active {
			l.bg = tk.SurfaceMuted
		}
		return l
	case Ghost:
		l := buttonLook{bg: tk.SurfaceMuted.Alpha(0), fg: tk.TextSecondary, border: ui.Transparent, opacity: 1}
		if active {
			l.bg, l.fg = tk.SurfaceMuted, tk.Foreground
		}
		return l
	case Danger:
		l := buttonLook{bg: tk.Surface, fg: tk.Danger, border: tk.Border, opacity: 1}
		if active {
			l.bg, l.border = tk.SurfaceMuted, tk.Danger
		}
		return l
	}
	l := buttonLook{bg: tk.Foreground, fg: tk.Background, border: tk.Foreground.Alpha(0), opacity: 1}
	switch {
	case pressed:
		l.opacity = primaryPress
	case hovered:
		l.opacity, l.shadow = primaryHover, true
	}
	return l
}

func Button(c *ui.Context, tk theme.Tokens, text string, o ButtonOpts) ui.Element {
	label := o.Label
	if label == "" {
		label = text
	}
	b := button(c, tk, o, label, pressScale, func() {
		if o.Icon != "" {
			ui.Icon(c, icons.Get(o.Icon)).Size(buttonIconSize, buttonIconSize)
		}
		if text != "" {
			ui.Text(c, text).SingleLine()
		}
	})
	return b.PaddingX(o.Size.paddingX())
}

func IconButton(c *ui.Context, tk theme.Tokens, icon, label string, o ButtonOpts) ui.Element {
	h := o.Size.height()
	b := button(c, tk, o, label, iconPressScale, func() {
		ui.Icon(c, icons.Get(icon)).Size(iconButtonGlyph, iconButtonGlyph)
	})
	return b.Size(h, h).Tooltip(label)
}

func button(c *ui.Context, tk theme.Tokens, o ButtonOpts, label string, squeeze float32, content func()) ui.Element {
	b := ui.ButtonBase(c).Label(label).Disabled(o.Disabled).MinHeight(o.Size.height()).Gap(buttonGap).
		Radius(theme.RadiusControl).FontSize(bodyFontSize).FontWeight(mediumWeight)
	if o.Loading {
		b.HandleInput(swallowActivation)
	}
	interactive := !o.Loading && !o.Disabled
	hovered := interactive && b.Hovered()
	pressed := interactive && b.Pressed()
	look := lookOf(tk, o.Variant, hovered, pressed)
	b.Background(b.AnimateColor("bg", look.bg, fast)).
		Border(1, b.AnimateColor("border", look.border, fast)).
		TextColor(b.AnimateColor("fg", look.fg, fast)).
		Opacity(b.AnimateTo("opacity", look.opacity, fast))
	if look.shadow {
		theme.WithShadow(b, tk.ShadowResting)
	}
	scale := float32(1)
	if pressed {
		scale = squeeze
	}
	scale = b.AnimateTo("press", scale, ui.Snappy)
	b.Scale(scale, scale)
	labelOpacity := float32(1)
	if o.Loading {
		labelOpacity = 0
	}
	labelOpacity = b.AnimateTo("label", labelOpacity, fast)
	return b.Children(func() {
		ui.Row(c).Gap(buttonGap).Center().Opacity(labelOpacity).Children(content)
		if o.Loading {
			spin(c, buttonIconSize, buttonSpinPeriod).Attach(ui.AnchorCenter, ui.AnchorCenter)
			ui.ButtonBase(c).Disabled(true).Role(ui.RoleNone).Absolute().Top(0).Right(0).Bottom(0).Left(0)
		}
	})
}

func swallowActivation(ev ui.InputEvent) bool {
	return (ev.Kind == ui.InputKeyDown || ev.Kind == ui.InputKeyUp) && ev.Mods == 0 &&
		(ev.Key == ui.KeyEnter || ev.Key == ui.KeySpace)
}

func quietButton(c *ui.Context, tk theme.Tokens, icon, label string, box, glyph, radius float32, on bool) ui.Element {
	b := ui.ButtonBase(c).Label(label).Size(box, box).Radius(radius)
	hovered, pressed := b.Hovered(), b.Pressed()
	bg, fg := tk.SurfaceMuted.Alpha(0), tk.TextMuted
	if hovered || pressed {
		bg, fg = tk.SurfaceMuted, tk.Foreground
	} else if on {
		fg = tk.Foreground
	}
	b.Background(b.AnimateColor("bg", bg, fast)).TextColor(b.AnimateColor("fg", fg, fast))
	scale := float32(1)
	if pressed {
		scale = iconPressScale
	}
	scale = b.AnimateTo("press", scale, ui.Snappy)
	return b.Scale(scale, scale).Children(func() {
		ui.Icon(c, icons.Get(icon)).Size(glyph, glyph)
	})
}
