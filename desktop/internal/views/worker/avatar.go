package worker

import (
	"cmp"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	avatarFieldGap = 16
	avatarNoteSize = 14
	pickerWidth    = 552
	pickerHeight   = 600
	pickerPad      = 24
	tileRing       = 2
	tileInset      = 2
	tileGap        = 8
	tileFace       = 48
	tileSize       = tileFace + 2*(tileRing+tileInset)
	packHeadSize   = 12
	packHeadTop    = 12
	packHeadBottom = 4
	randomGlyph    = 18
	randomDash     = 1
	pickerOverscan = 2
	packsStale     = 30 * time.Second
)

type pickerState struct {
	open   bool
	reveal bool
	list   ui.ListState
}

type avatarRetryKey struct{}

func (p *Panel) avatarField(c *ui.Context, tk theme.Tokens, box ui.Element, es *editState, w api.Worker) {
	packs := p.s.Avatars
	ui.Local(box, avatarRetryKey{}, func() bool {
		if !packs.Loading && (packs.Err != nil || packs.Loaded && packs.Stale(packsStale)) {
			packs.Invalidate()
		}
		return true
	})
	packs.Ensure()
	filled := model.FilledPacks(packs.Data)
	name := cmp.Or(strings.TrimSpace(es.form.Name), w.Name)
	kit.Field(c, tk, locales.WorkersFormAvatar(), "", "", func() {
		ui.Row(c).MinWidth(0).AlignItems(ui.Center).Gap(avatarFieldGap).Children(func() {
			kit.WorkerAvatar(c, tk, w.ID, name, "", p.s.AvatarImage(es.form.Avatar), kit.AvatarLG, false)
			switch {
			case packs.Err != nil && !packs.Loaded:
				ui.Text(c, locales.WorkersFormAvatarLoadError()).FontSize(avatarNoteSize).TextColor(tk.Danger).MinWidth(0)
			case !packs.Loaded:
			case len(filled) == 0:
				ui.Text(c, locales.WorkersFormAvatarNone()).FontSize(avatarNoteSize).TextColor(tk.TextSecondary).MinWidth(0)
			default:
				kit.Button(c, tk, locales.WorkersFormAvatarChange(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM}).
					OnClick(func() { es.picker.open, es.picker.reveal = true, true })
			}
		})
	})
	p.avatarPicker(c, tk, es, w, name, filled)
}

func (p *Panel) avatarPicker(c *ui.Context, tk theme.Tokens, es *editState, w api.Worker, name string, packs []api.AvatarPack) {
	pk := &es.picker
	if !pk.open {
		return
	}
	p.s.Workers.Ensure()
	cols := model.PickerColumns(kit.DialogWidth(c, pickerWidth)-2*pickerPad, tileSize, tileGap)
	rows := model.PickerRows(packs, cols)
	choose := func(ref string) {
		es.form.Avatar = ref
		pk.open = false
	}
	if pk.reveal {
		pk.reveal = false
		pk.list.ScrollTo(max(0, model.PickerRowOf(rows, es.form.Avatar)), ui.Center)
	}
	random := func() {
		used := model.UsedAvatars(p.s.Workers.Data, w.ID)
		if es.form.Avatar != "" {
			used[es.form.Avatar] = true
		}
		choose(model.RandomAvatar(packs, used, rand.IntN))
	}
	kit.Dialog(c, tk, &pk.open, kit.DialogOpts{
		Title:       locales.WorkersFormAvatarDialogTitle(),
		Description: locales.WorkersFormAvatarDialogDescription(name),
		Width:       pickerWidth,
		Height:      pickerHeight,
		Flush:       true,
	}, func() {
		lang := i18n.Locale()
		first, last := pk.list.Visible()
		ui.List(c, &pk.list, len(rows), func(i int) {
			row := rows[i]
			load := i >= first-pickerOverscan && i <= last+pickerOverscan
			if row.Heading {
				pack := packs[row.Pack]
				ui.Text(c, cmp.Or(l10n.Localized(pack.Name, lang), pack.ID)).FontSize(packHeadSize).FontWeight(mediumWeight).
					TextColor(tk.TextSecondary).Role(ui.RoleHeading).Level(3).Padding(packHeadTop, 0, packHeadBottom, 0)
				return
			}
			ui.Row(c).MinWidth(0).Gap(tileGap).PaddingY(tileGap / 2).Children(func() {
				for _, tile := range row.Tiles {
					ui.Box(c).Key(tile).Grow(1).Basis(0).MinWidth(0).Center().Children(func() {
						if tile.Kind == model.TileRandom {
							randomTile(c, tk, random)
							return
						}
						p.avatarTile(c, tk, w.ID, name, tile.Ref, tile.Ref == es.form.Avatar, load, choose)
					})
				}
				for k := len(row.Tiles); k < cols; k++ {
					ui.Box(c).Key(k).Grow(1).Basis(0).MinWidth(0)
				}
			})
		}).Grow(1).MinHeight(0).Padding(0, pickerPad).Margin(pickerPad-tileGap/2, 0).Label(locales.WorkersFormAvatarDialogTitle())
	})
}

func (p *Panel) avatarTile(c *ui.Context, tk theme.Tokens, id, name, ref string, selected, load bool, choose func(ref string)) {
	label := locales.WorkersFormAvatarInitials()
	if ref != "" {
		label = locales.WorkersFormAvatarOption(model.AvatarLabel(ref))
	}
	b := ui.ButtonBase(c).Label(label).Role(ui.RoleRadio).Checked(selected).Size(tileSize, tileSize).Shrink(0).Center().
		Radius(theme.RadiusPill)
	ring := tk.BorderStrong.Alpha(0)
	switch {
	case selected:
		ring = tk.Accent
	case b.Hovered() || b.Pressed():
		ring = tk.BorderStrong
	}
	var img ui.ImageSource
	if load {
		img = p.s.AvatarImage(ref)
	}
	b.Border(tileRing, b.AnimateColor("ring", ring, fast)).OnClick(func() { choose(ref) }).Children(func() {
		kit.WorkerAvatar(c, tk, id, name, "", img, kit.AvatarLG, false)
	})
	kit.Tooltip(c, tk, b, label)
}

func randomTile(c *ui.Context, tk theme.Tokens, random func()) {
	label := locales.WorkersFormAvatarRandom()
	b := ui.ButtonBase(c).Label(label).Size(tileSize, tileSize).Shrink(0).Center().Radius(theme.RadiusPill)
	ring := tk.BorderStrong.Alpha(0)
	if b.Hovered() || b.Pressed() {
		ring = tk.BorderStrong
	}
	b.Border(tileRing, b.AnimateColor("ring", ring, fast)).OnClick(random).Children(func() {
		ui.Box(c).Size(tileFace, tileFace).Center().Radius(theme.RadiusPill).Border(randomDash, tk.BorderStrong).
			BorderStyle(ui.BorderDashed).TextColor(tk.TextSecondary).Children(func() {
			ui.Icon(c, icons.Get("shuffle")).Size(randomGlyph, randomGlyph)
		})
	})
	kit.Tooltip(c, tk, b, label)
}
