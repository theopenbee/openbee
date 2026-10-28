package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	switchTrackWidth  = 42
	switchTrackHeight = 24
	switchPadding     = 3
	switchThumb       = 18
	switchTravel      = 18
	switchStretch     = 5
	switchLabelGap    = 12
	switchRowGap      = 16
	rowCopyGap        = 2
	checkVisual       = 18
	checkInset        = (theme.ControlMD - checkVisual) / 2
	checkStroke       = 1.75
	checkPressScale   = 0.95
	checkFillFrom     = 0.6
	checkCopyStart    = 1
	checkCopyTop      = (theme.ControlMD - bodyFontSize*theme.BodyLineHeight) / 2
)

var (
	switchGlide = ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond}}
	checkPath   = [...]ui.Point{{X: 4.25, Y: 9.25}, {X: 7.25, Y: 12.25}, {X: 13.75, Y: 5.75}}
)

func SwitchRow(c *ui.Context, tk theme.Tokens, on *bool, title, description string, disabled bool) ui.Element {
	s := ui.SwitchBase(c, on).Disabled(disabled).Label(title).FontSize(bodyFontSize).AlignItems(ui.Center)
	if description == "" {
		s.MinHeight(theme.ControlMD).Gap(switchLabelGap)
	} else {
		s.Description(description).Justify(ui.SpaceBetween).Gap(switchRowGap)
	}
	return s.Children(func() {
		if description == "" {
			switchTrack(c, tk, s, *on, disabled)
			if title != "" {
				ui.Text(c, title).TextColor(tk.Foreground)
			}
			return
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(rowCopyGap).Children(func() {
			ui.Text(c, title).FontWeight(mediumWeight).TextColor(tk.Foreground)
			ui.Text(c, description).TextColor(tk.TextSecondary)
		})
		switchTrack(c, tk, s, *on, disabled)
	})
}

func switchTrack(c *ui.Context, tk theme.Tokens, s ui.Element, on, disabled bool) {
	bg := tk.ControlTrack
	switch {
	case on:
		bg = tk.ControlOn
	case s.Hovered() && !disabled:
		bg = tk.ControlTrackHover
	}
	stretch := float32(0)
	if s.Pressed() && !disabled {
		stretch = switchStretch
	}
	x := float32(0)
	if on {
		x = switchTravel - stretch
	}
	track := ui.Row(c).Size(switchTrackWidth, switchTrackHeight).Shrink(0).Padding(switchPadding).
		Radius(theme.RadiusPill).Background(s.AnimateColor("track", bg, standard))
	track.Children(func() {
		thumb := ui.Box(c).Height(switchThumb).Radius(theme.RadiusPill).Background(tk.ControlThumb)
		thumb.Width(thumb.AnimateTo("width", switchThumb+stretch, ui.Snappy)).
			Translate(thumb.AnimateTo("x", x, switchGlide), 0)
		theme.WithShadow(thumb, tk.ShadowThumb)
	})
}

func CheckboxRow(c *ui.Context, tk theme.Tokens, on *bool, title, description string) ui.Element {
	name := title
	if name == "" {
		name = locales.ArcCheckboxLabel()
	}
	b := ui.CheckboxBase(c, on).Label(name).AlignItems(ui.Start)
	if description != "" {
		b.Description(description)
	}
	border := tk.BorderStrong
	switch {
	case *on:
		border = tk.ControlOn
	case b.Hovered():
		border = tk.TextMuted
	}
	scale := float32(1)
	if b.Pressed() {
		scale = checkPressScale
	}
	shown := float32(0)
	if *on {
		shown = 1
	}
	return b.Children(func() {
		ui.Box(c).Size(theme.ControlMD, theme.ControlMD).Shrink(0).Center().Children(func() {
			radius := theme.Concentric(theme.RadiusControl, checkInset)
			box := ui.Box(c).Size(checkVisual, checkVisual).Radius(radius).Background(tk.Surface).
				Border(1, b.AnimateColor("border", border, fast))
			pressed := box.AnimateTo("press", scale, instant)
			box.Scale(pressed, pressed)
			fill := box.AnimateTo("fill", shown, ui.Snappy)
			mark := box.AnimateTo("mark", shown, ui.Snappy)
			glyph := tk.ControlThumb
			box.Children(func() {
				fillScale := checkFillFrom + (1-checkFillFrom)*fill
				ui.Box(c).Absolute().Top(0).Right(0).Bottom(0).Left(0).Radius(radius).Background(tk.ControlOn).
					Opacity(min(1, fill)).Scale(fillScale, fillScale)
			})
			box.DrawOver(func(p *ui.Painter, r ui.Rect) {
				if mark <= 0 {
					return
				}
				path := new(ui.Path).MoveTo(r.X+checkPath[0].X, r.Y+checkPath[0].Y)
				for _, pt := range checkPath[1:] {
					path.LineTo(r.X+pt.X, r.Y+pt.Y)
				}
				path.Join(ui.RoundJoin).Cap(ui.RoundCap).Trim(0, min(1, mark))
				p.StrokePath(path, checkStroke, glyph.Alpha(min(1, mark)))
			})
		})
		if title == "" && description == "" {
			return
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(rowCopyGap).Padding(checkCopyTop, 0, 0, checkCopyStart).Children(func() {
			if title != "" {
				ui.Text(c, title).FontSize(bodyFontSize).FontWeight(mediumWeight).TextColor(tk.Foreground)
			}
			if description != "" {
				ui.Text(c, description).FontSize(smallFontSize).TextColor(tk.TextMuted)
			}
		})
	})
}
