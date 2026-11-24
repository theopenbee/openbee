package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	paletteWidth       = 576
	paletteTopShare    = 0.12
	paletteRise        = 6
	paletteSearchMinH  = 54
	paletteSearchPadX  = 14
	paletteSearchPadY  = 8
	paletteSearchGap   = 10
	paletteSearchGlyph = 18
	paletteListMax     = 342
	paletteListPadX    = 8
	paletteListPadY    = 6
	paletteGroupGap    = 6
	paletteHeadingPadX = 10
	paletteHeadingTop  = 8
	paletteHeadingEnd  = 4
	paletteRowHeight   = 38
	paletteRowPadX     = 10
	paletteRowGap      = 10
	paletteGlyph       = 16
	paletteCopyGap     = 8
	paletteChipHeight  = 24
	paletteChipPadX    = 6
	paletteEmptyPadTop = 26
	paletteEmptyPadEnd = 28
	paletteEmptyGap    = 4
	paletteEmptyGlyph  = 20
	paletteEmptyBox    = 32
	paletteEmptyIcon   = 5
)

type PaletteItem struct {
	ID          string
	Label       string
	Description string
	Group       string
	Icon        string
	Glyph       *ui.SVG
	Logo        bool
	Keywords    []string
	Keys        []string
}

type PaletteState struct {
	Query  string
	active int
	seen   string
	reveal bool
}

func (st *PaletteState) Reset() {
	*st = PaletteState{}
}

type PaletteOpts struct {
	Label       string
	Placeholder string
	Keys        []string
}

type paletteGroup struct {
	name  string
	items []PaletteItem
}

func PaletteResults(items []PaletteItem, query string) []PaletteItem {
	search := make([]model.Searchable, len(items))
	for i, it := range items {
		search[i] = model.Searchable{Label: it.Label, Description: it.Description, Keywords: it.Keywords}
	}
	var groups []paletteGroup
	for _, i := range model.RankSearch(search, query) {
		it := items[i]
		at := -1
		for g := range groups {
			if groups[g].name == it.Group {
				at = g
				break
			}
		}
		if at < 0 {
			groups = append(groups, paletteGroup{name: it.Group})
			at = len(groups) - 1
		}
		groups[at].items = append(groups[at].items, it)
	}
	var out []PaletteItem
	for _, g := range groups {
		out = append(out, g.items...)
	}
	return out
}

func CommandPalette(c *ui.Context, tk theme.Tokens, open *bool, st *PaletteState, items []PaletteItem, o PaletteOpts, choose func(PaletteItem)) ui.Element {
	if !*open {
		return ui.Element{}
	}
	results := PaletteResults(items, st.Query)
	if st.Query != st.seen {
		st.seen, st.active = st.Query, 0
	}
	st.active = min(st.active, max(len(results)-1, 0))
	pick := func(it PaletteItem) {
		*open = false
		choose(it)
	}
	w, h := c.Size()
	top := c.TitleBar().Height
	return ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(tk.Scrim.Alpha(fadeIn(backdrop))).Justify(ui.Start).Padding(top+h*paletteTopShare, 0, 0, 0)
		floatingPanel(panel, tk, theme.RadiusPanel).Label(o.Label).Width(min(paletteWidth, w-2*dialogInset)).
			MaxHeight(h - top - 2*dialogInset).Clip()
		appear(panel, durationFast, paletteRise, popoverScale)
		panel.Children(func() {
			paletteSearch(c, tk, open, st, results, o, pick)
			paletteList(c, tk, st, results, pick)
		})
	})
}

func paletteSearch(c *ui.Context, tk theme.Tokens, open *bool, st *PaletteState, results []PaletteItem, o PaletteOpts, pick func(PaletteItem)) {
	row := ui.Row(c).Shrink(0).MinHeight(paletteSearchMinH).AlignItems(ui.Center).Gap(paletteSearchGap).
		Padding(paletteSearchPadY, paletteSearchPadX).BorderWidth(0, 0, 1, 0).BorderColor(tk.BorderSubtle)
	bg := tk.SurfaceMuted.Alpha(0)
	if row.FocusWithin() {
		bg = tk.SurfaceMuted
	}
	row.Background(row.AnimateColor("bg", bg, fast)).Children(func() {
		ui.Icon(c, icons.Get("search")).Size(paletteSearchGlyph, paletteSearchGlyph).Shrink(0).TextColor(tk.TextMuted)
		ui.TextInputBase(c, &st.Query).Label(o.Label).Placeholder(o.Placeholder).FontSize(bodyFontSize).
			Grow(1).MinWidth(0).AlignSelf(ui.Stretch).FocusRing(false).AutoFocus().
			OnTextKey(func(k ui.TextKey) bool { return paletteKey(st, results, k, pick) })
		if st.Query != "" {
			clearSearch(c, tk).OnClick(func() { st.Query = "" })
		}
		paletteChip(c, tk, locales.ArcCommandPaletteClose(), "Esc").OnClick(func() { *open = false })
		if len(o.Keys) > 0 {
			Keys(c, tk, o.Keys)
		}
	})
}

func paletteKey(st *PaletteState, results []PaletteItem, k ui.TextKey, pick func(PaletteItem)) bool {
	if k.Mods != 0 {
		return false
	}
	switch k.Key {
	case ui.KeyDown:
		st.active, st.reveal = min(st.active+1, max(len(results)-1, 0)), true
	case ui.KeyUp:
		st.active, st.reveal = max(st.active-1, 0), true
	case ui.KeyEnter:
		if len(results) > 0 {
			pick(results[st.active])
		}
	case ui.KeyEscape:
		if k.Text() == "" {
			return false
		}
		k.SetText("")
	default:
		return false
	}
	return true
}

func paletteChip(c *ui.Context, tk theme.Tokens, label, text string) ui.Element {
	b := ui.ButtonBase(c).Label(label).Height(paletteChipHeight).PaddingX(paletteChipPadX).Shrink(0).
		Radius(theme.Concentric(theme.RadiusInset, keyCapInset)).KeepFocus()
	bg, fg := tk.Surface.Alpha(0), tk.TextMuted
	if b.Hovered() || b.Pressed() {
		bg, fg = tk.Surface, tk.Foreground
	}
	return b.Background(b.AnimateColor("bg", bg, fast)).Border(1, tk.BorderSubtle).TextColor(b.AnimateColor("fg", fg, fast)).
		Children(func() {
			ui.Text(c, text).FontSize(smallFontSize).FontWeight(mediumWeight).LineHeight(1).SingleLine()
		})
}

func paletteList(c *ui.Context, tk theme.Tokens, st *PaletteState, results []PaletteItem, pick func(PaletteItem)) {
	if len(results) == 0 {
		paletteEmpty(c, tk, st.Query)
		return
	}
	ui.Scroll(c).MaxHeight(paletteListMax).Shrink(1).MinHeight(0).Padding(paletteListPadY, paletteListPadX).Role(ui.RoleList).Children(func() {
		group := ""
		for i, it := range results {
			if i == 0 || it.Group != group {
				group = it.Group
				heading := ui.Text(c, group).FontSize(smallFontSize).FontWeight(mediumWeight).TextColor(tk.TextMuted).
					Padding(paletteHeadingTop, paletteHeadingPadX, paletteHeadingEnd, paletteHeadingPadX).Role(ui.RoleHeading).Level(3)
				if i > 0 {
					heading.Margin(paletteGroupGap, 0, 0, 0)
				}
			}
			paletteRow(c, tk, st, i, it, pick)
		}
	})
	st.reveal = false
}

func paletteRow(c *ui.Context, tk theme.Tokens, st *PaletteState, i int, it PaletteItem, pick func(PaletteItem)) {
	active := i == st.active
	b := ui.ButtonBase(c).Key(it.ID).Label(it.Label).Selected(active).KeepFocus().Height(paletteRowHeight).Shrink(0).
		Padding(0, paletteRowPadX).Gap(paletteRowGap).AlignItems(ui.Center).Justify(ui.Start).
		Radius(theme.Concentric(theme.RadiusPanel, paletteListPadX+paletteRowPadX)).Cursor(ui.CursorPointer)
	bg, glyph := tk.SurfaceMuted.Alpha(0), tk.TextMuted
	if active || b.Hovered() || b.Pressed() {
		bg = tk.SurfaceMuted
	}
	if active {
		glyph = tk.Foreground
	}
	if it.Description != "" {
		b.Description(it.Description)
	}
	icon := it.Glyph
	if icon == nil && it.Icon != "" {
		icon = icons.Get(it.Icon)
	}
	b.Background(b.AnimateColor("bg", bg, fast)).OnClick(func() { pick(it) }).Children(func() {
		switch {
		case it.Logo:
			ui.Image(c, icons.LogoMarkSmall(tk)).Size(paletteGlyph, paletteGlyph).Shrink(0)
		case icon != nil:
			ui.Icon(c, icon).Size(paletteGlyph, paletteGlyph).Shrink(0).TextColor(b.AnimateColor("glyph", glyph, fast))
		}
		ui.Row(c).Grow(1).MinWidth(0).Gap(paletteCopyGap).AlignItems(ui.Baseline).Children(func() {
			ui.Text(c, it.Label).FontSize(bodyFontSize).TextColor(tk.Foreground).SingleLine().Shrink(0).MaxWidthPercent(100)
			if it.Description != "" {
				ui.Text(c, it.Description).FontSize(smallFontSize).TextColor(tk.TextSecondary).SingleLine().MinWidth(0).Shrink(1)
			}
		})
		if len(it.Keys) > 0 {
			Keys(c, tk, it.Keys)
		}
	})
	if active && st.reveal {
		b.ScrollIntoView()
	}
}

func paletteEmpty(c *ui.Context, tk theme.Tokens, query string) {
	ui.Column(c).AlignItems(ui.Center).Gap(paletteEmptyGap).Padding(paletteEmptyPadTop, paletteRowPadX, paletteEmptyPadEnd, paletteRowPadX).
		Role(ui.RoleStatus).Children(func() {
		ui.Box(c).Size(paletteEmptyBox, paletteEmptyBox).Center().Margin(0, 0, paletteEmptyIcon, 0).Children(func() {
			ui.Icon(c, icons.Get("search")).Size(paletteEmptyGlyph, paletteEmptyGlyph).TextColor(tk.TextMuted)
		})
		ui.Text(c, locales.ArcCommandPaletteEmpty(query)).FontSize(bodyFontSize).FontWeight(mediumWeight).SingleLine().MaxWidthPercent(100)
		ui.Text(c, locales.ArcCommandPaletteEmptyHint()).FontSize(smallFontSize).TextColor(tk.TextSecondary)
	})
}
