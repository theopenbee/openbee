package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

type Tone int

const (
	ToneNeutral Tone = iota
	ToneInfo
	ToneSuccess
	ToneWarning
	ToneDanger
)

type AlertOpts struct {
	Tone        Tone
	Title, Body string
	ActionLabel string
	OnAction    func()
	OnDismiss   func()
}

const (
	alertPadding     = 16
	alertGap         = 12
	alertGlyph       = 18
	alertBodyGap     = 4
	alertActionGap   = 12
	alertDismissBox  = 28
	alertDismissIcon = 16
)

func (t Tone) color(tk theme.Tokens) ui.Color {
	switch t {
	case ToneInfo:
		return tk.Accent
	case ToneSuccess:
		return tk.Success
	case ToneWarning:
		return tk.Warning
	case ToneDanger:
		return tk.Danger
	}
	return tk.TextSecondary
}

func (t Tone) icon() string {
	switch t {
	case ToneSuccess:
		return "circle-check"
	case ToneWarning:
		return "triangle-alert"
	case ToneDanger:
		return "circle-x"
	}
	return "info"
}

func Alert(c *ui.Context, tk theme.Tokens, o AlertOpts) ui.Element {
	a := ui.Row(c).AlignItems(ui.Start).Gap(alertGap).Padding(alertPadding).Role(ui.RoleStatus).
		Radius(theme.RadiusPanel).Background(tk.Surface).Border(1, tk.Border)
	theme.WithShadow(a, tk.ShadowResting)
	return a.Children(func() {
		ui.Icon(c, icons.Get(o.Tone.icon())).Size(alertGlyph, alertGlyph).TextColor(o.Tone.color(tk)).Margin(1, 0, 0, 0)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, o.Title).FontSize(bodyFontSize).FontWeight(mediumWeight)
			if o.Body != "" {
				ui.Text(c, o.Body).FontSize(smallFontSize).TextColor(tk.TextSecondary).Margin(alertBodyGap, 0, 0, 0)
			}
			if o.ActionLabel != "" && o.OnAction != nil {
				Button(c, tk, o.ActionLabel, ButtonOpts{Variant: Secondary, Size: SizeSM}).
					AlignSelf(ui.Start).Margin(alertActionGap, 0, 0, 0).OnClick(o.OnAction)
			}
		})
		if o.OnDismiss != nil {
			quietButton(c, tk, "x", locales.ArcAlertDismiss(o.Title), alertDismissBox, alertDismissIcon, theme.RadiusPill, false).
				Margin(-5, -6, -5, 0).OnClick(o.OnDismiss)
		}
	})
}
