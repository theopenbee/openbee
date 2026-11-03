package chat

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	chipsGap        = 8
	chipsPadTop     = 4
	chipsPadX       = 8
	chipsPadBottom  = 8
	pillMinHeight   = 32
	pillPadY        = 4
	pillPadEnd      = 4
	pillPadStart    = 12
	pillGap         = 8
	pillFont        = 12
	pillName        = 160
	pillNameWide    = 224
	pillRemove      = 24
	pillRemoveGlyph = 16
)

func chips(c *ui.Context, tk theme.Tokens, cm *composerState, wide bool) {
	limit := float32(pillName)
	if wide {
		limit = pillNameWide
	}
	ui.Row(c).Wrap().Gap(chipsGap).Padding(chipsPadTop, chipsPadX, chipsPadBottom, chipsPadX).
		Role(ui.RoleList).Label(locales.ChatAttachments()).Children(func() {
		for _, path := range cm.files {
			pendingChip(c, tk, cm, path, limit)
		}
	})
}

func pendingChip(c *ui.Context, tk theme.Tokens, cm *composerState, path string, limit float32) {
	name := mediaBase(path)
	ui.Row(c).Key(path).Role(ui.RoleListItem).MinHeight(pillMinHeight).MinWidth(0).MaxWidthPercent(100).
		AlignItems(ui.Center).Gap(pillGap).Padding(pillPadY, pillPadEnd, pillPadY, pillPadStart).Radius(theme.RadiusPill).
		Border(1, tk.Border).Background(tk.SurfaceMuted).FontSize(pillFont).TextColor(tk.Foreground).Children(func() {
		ui.Icon(c, icons.Get(categoryIcons[fileCategory(name)])).Size(chipGlyph, chipGlyph).Shrink(0).TextColor(tk.TextSecondary)
		ui.Text(c, name).SingleLine().MinWidth(0).MaxWidth(limit)
		removeChip(c, tk, cm, path, name)
	})
}

func removeChip(c *ui.Context, tk theme.Tokens, cm *composerState, path, name string) {
	b := ui.ButtonBase(c).Label(locales.ChatRemoveNamedAttachment(name)).Size(pillRemove, pillRemove).Shrink(0).
		Radius(theme.RadiusPill)
	bg, fg := tk.Surface.Alpha(0), tk.TextSecondary
	if b.Hovered() || b.Pressed() {
		bg, fg = tk.Surface, tk.Foreground
	}
	b.Background(b.AnimateColor("bg", bg, colorFade)).TextColor(b.AnimateColor("fg", fg, colorFade)).
		OnClick(func() { cm.removeFile(path) }).Children(func() {
		ui.Icon(c, icons.Get("x")).Size(pillRemoveGlyph, pillRemoveGlyph)
	})
}
