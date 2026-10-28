package chat

import (
	"context"
	"errors"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	headerPadNarrow = 12
	headerPadWide   = 20
	headerGap       = 12
	backBox         = 40
	backGlyph       = 20
	titleSize       = 16
	statusSize      = 12
	titlePadX       = 8
	actionsGap      = 4
)

func (v *View) headerBar(c *ui.Context, tk theme.Tokens, content func()) {
	w, _ := c.Size()
	narrow := w < theme.BreakTwoPane
	pad := float32(headerPadWide)
	start := pad
	if narrow {
		pad = headerPadNarrow
		start = max(pad, c.TitleBar().Left)
	}
	ui.Row(c).Height(theme.HeaderHeight).Shrink(0).Gap(headerGap).PaddingStart(start).PaddingEnd(pad).
		BorderWidth(0, 0, 1, 0).BorderColor(tk.Border).DragWindow().Children(func() {
		if narrow {
			v.backButton(c, tk)
		}
		if content != nil {
			content()
		}
	})
}

func (v *View) backButton(c *ui.Context, tk theme.Tokens) {
	b := ui.ButtonBase(c).Label(locales.ChatBack()).Role(ui.RoleLink).Size(backBox, backBox).Shrink(0).Radius(theme.RadiusControl)
	bg, fg := tk.SurfaceMuted.Alpha(0), tk.TextSecondary
	if b.Hovered() || b.Pressed() {
		bg, fg = tk.SurfaceMuted, tk.Foreground
	}
	b.Background(b.AnimateColor("bg", bg, colorFade)).TextColor(b.AnimateColor("fg", fg, colorFade)).
		OnClick(func() { v.nav.Push(route.ChatPath) }).Children(func() {
		ui.Icon(c, icons.Get("chevron-left")).Size(backGlyph, backGlyph)
	})
}

func (v *View) header(c *ui.Context, tk theme.Tokens, p pane, panelOpen bool) {
	kind, ahead := model.ChatStatus(v.activity(p.cid), p.worker)
	busy := kind == model.StatusRunning || kind == model.StatusQueued
	v.headerBar(c, tk, func() {
		if p.worker == nil {
			kit.BeeAvatar(c, tk, kit.AvatarMD)
		} else {
			kit.WorkerAvatar(c, tk, p.worker.ID, p.worker.Name, p.worker.Status, kit.AvatarMD, true)
		}
		ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Start).Children(func() {
			if p.worker == nil {
				ui.Text(c, locales.ChatBeeLabel()).FontSize(titleSize).FontWeight(mediumWeight).SingleLine().MaxWidthPercent(100).
					Role(ui.RoleHeading).Level(1)
			} else {
				v.panelToggle(c, tk, p, panelOpen)
			}
			ui.Text(c, statusText(kind, ahead)).FontSize(statusSize).TextColor(tk.TextSecondary).SingleLine().MaxWidthPercent(100)
		})
		ui.Row(c).Shrink(0).Gap(actionsGap).Children(func() {
			if p.worker != nil && busy {
				kit.Button(c, tk, locales.ChatStop(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM, Icon: "square", Loading: p.ch.Stopping}).
					OnClick(func() { v.stop(p) })
			}
			kit.IconButton(c, tk, "square-pen", locales.ChatNewConversation(), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).
				OnClick(p.cs.reset.show)
		})
	})
}

func (v *View) panelToggle(c *ui.Context, tk theme.Tokens, p pane, open bool) {
	tip := locales.ChatOpenPanel()
	if open {
		tip = locales.ChatClosePanel()
	}
	name := p.worker.Name
	b := ui.ButtonBase(c).Label(name).Expanded(open).MinWidth(0).MaxWidthPercent(100).PaddingX(titlePadX).MarginX(-titlePadX).
		Radius(theme.RadiusControl).Role(ui.RoleHeading).Level(1)
	bg := tk.SurfaceMuted.Alpha(0)
	if b.Hovered() || b.Pressed() {
		bg = tk.SurfaceMuted
	}
	b.Background(b.AnimateColor("bg", bg, colorFade)).OnClick(func() { v.togglePanel(p.cid) }).Children(func() {
		ui.Text(c, name).FontSize(titleSize).FontWeight(mediumWeight).TextColor(tk.Foreground).SingleLine().MinWidth(0)
	})
	kit.Tooltip(c, tk, b, tip)
}

func (v *View) stop(p pane) {
	p.ch.Stop(func(err error) {
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}
		v.s.Toast(kit.Toast{Tone: kit.ToneDanger, Title: locales.ChatStopFailed(), Description: l10n.ErrorText(err)})
	})
}

func statusText(kind model.StatusKind, ahead int) string {
	switch kind {
	case model.StatusQueued:
		return locales.ChatStatusQueued(ahead)
	case model.StatusRunning:
		return locales.ChatStatusRunning()
	case model.StatusError:
		return locales.StatusesError()
	}
	return locales.StatusesIdle()
}
